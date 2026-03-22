package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/updater"
)

var (
	updateCheck   bool
	updateYes     bool
	updateVersion string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update tsk to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Fetch release info
		var release *updater.Release
		var err error

		if updateVersion != "" {
			// Fetch specific version
			fmt.Fprintf(os.Stderr, "Checking for version %s...\n", updateVersion)
			release, err = updater.FetchReleaseByTag(ctx, updateVersion)
		} else {
			// Fetch latest
			fmt.Fprintf(os.Stderr, "Checking for updates...\n")
			release, err = updater.FetchLatestRelease(ctx)
		}

		if err != nil {
			return fmt.Errorf("failed to fetch release: %w", err)
		}

		// Compare versions
		isNewer := updater.IsNewer(release.TagName, appVersion)
		if !isNewer && updateVersion == "" {
			fmt.Fprintf(os.Stderr, "Already up to date (current: %s)\n", appVersion)
			return nil
		}

		// If only checking, print and exit
		if updateCheck {
			fmt.Fprintf(os.Stderr, "Current: %s\n", appVersion)
			fmt.Fprintf(os.Stderr, "Latest:  %s\n", release.TagName)
			if isNewer {
				fmt.Fprintf(os.Stderr, "\nUpdate available! Run 'tsk update' to install.\n")
			} else {
				fmt.Fprintf(os.Stderr, "\nAlready up to date.\n")
			}
			return nil
		}

		// Confirmation prompt (unless --yes)
		if !updateYes {
			prompt := fmt.Sprintf("Update tsk from %s to %s? [y/N]: ", appVersion, release.TagName)
			fmt.Fprint(os.Stderr, prompt)

			scanner := bufio.NewScanner(os.Stdin)
			if !scanner.Scan() {
				return fmt.Errorf("cancelled")
			}
			response := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if response != "y" && response != "yes" {
				fmt.Fprintf(os.Stderr, "Cancelled.\n")
				return nil
			}
		}

		// Find asset for current platform
		fmt.Fprintf(os.Stderr, "Looking for binary for %s/%s...\n", runtime.GOOS, runtime.GOARCH)
		assetURL := updater.FindAsset(release, runtime.GOOS, runtime.GOARCH)
		if assetURL == "" {
			return fmt.Errorf("no binary available for %s/%s", runtime.GOOS, runtime.GOARCH)
		}

		// Download asset
		assetName := updater.AssetName(release.TagName, runtime.GOOS, runtime.GOARCH)
		fmt.Fprintf(os.Stderr, "Downloading %s...\n", assetName)

		archiveData, err := updater.DownloadAsset(ctx, assetURL, func(downloaded, total int64) {
			if total > 0 {
				pct := (downloaded * 100) / total
				fmt.Fprintf(os.Stderr, "\r  [%3d%%] %s / %s",
					pct,
					formatBytes(downloaded),
					formatBytes(total))
			}
		})
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "\n")

		// Verify checksum
		fmt.Fprintf(os.Stderr, "Verifying checksum...\n")
		checksums, err := updater.FetchChecksums(ctx, release.TagName)
		if err != nil {
			return fmt.Errorf("fetch checksums: %w", err)
		}

		expectedChecksum, ok := checksums[assetName]
		if !ok {
			return fmt.Errorf("checksum not found for %s", assetName)
		}

		if err := updater.VerifyChecksum(archiveData, expectedChecksum); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Checksum verified.\n")

		// Extract binary
		fmt.Fprintf(os.Stderr, "Extracting binary...\n")
		binaryData, err := updater.ExtractBinary(archiveData, assetName)
		if err != nil {
			return fmt.Errorf("extract failed: %w", err)
		}

		// Replace binary
		fmt.Fprintf(os.Stderr, "Installing...\n")
		selfPath, err := updater.SelfPath()
		if err != nil {
			return fmt.Errorf("determine binary path: %w", err)
		}

		if err := updater.ReplaceBinary(selfPath, binaryData); err != nil {
			return fmt.Errorf("replace binary: %w", err)
		}

		fmt.Fprintf(os.Stderr, "\nUpdated tsk to %s\n", release.TagName)
		fmt.Fprintf(os.Stderr, "Run 'tsk version' to verify.\n")

		return nil
	},
}

// formatBytes formats bytes into human-readable format
func formatBytes(b int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	f := float64(b)
	for _, unit := range units {
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", f, unit)
		}
		f /= 1024
	}
	return fmt.Sprintf("%.1f TB", f)
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "Check for updates without installing")
	updateCmd.Flags().BoolVarP(&updateYes, "yes", "y", false, "Skip confirmation prompt")
	updateCmd.Flags().StringVar(&updateVersion, "version", "", "Install a specific version (e.g. v1.3.0)")
	rootCmd.AddCommand(updateCmd)
}
