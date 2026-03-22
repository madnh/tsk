package updater

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"
)

const (
	githubAPI = "https://api.github.com/repos/madnh/tsk/releases"
	userAgent = "tsk-updater"
)

// Release represents the GitHub API response for /releases/latest.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset from GitHub.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// FetchLatestVersion queries the GitHub API and returns the latest tag name (e.g. "v1.2.3").
func FetchLatestVersion(ctx context.Context) (string, error) {
	release, err := FetchLatestRelease(ctx)
	if err != nil {
		return "", err
	}
	return release.TagName, nil
}

// FetchLatestRelease fetches the full Release struct from GitHub API.
func FetchLatestRelease(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubAPI+"/latest", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &release, nil
}

// FetchReleaseByTag fetches a specific release by tag name.
func FetchReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	url := fmt.Sprintf("%s/tags/%s", githubAPI, tag)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &release, nil
}

// IsNewer reports whether candidate is strictly newer than current.
// Both must be semver strings with leading "v".
// Returns false (not newer) on any parse error or if current == "dev".
func IsNewer(candidate, current string) bool {
	if current == "dev" {
		return false
	}
	cmp := semver.Compare(candidate, current)
	return cmp > 0
}

// AssetName constructs the expected archive filename for the current platform.
// version is without the "v" prefix (GoReleaser archive naming convention).
func AssetName(version, goos, goarch string) string {
	version = strings.TrimPrefix(version, "v")
	if goos == "windows" {
		return fmt.Sprintf("tsk_%s_%s_%s.zip", version, goos, goarch)
	}
	return fmt.Sprintf("tsk_%s_%s_%s.tar.gz", version, goos, goarch)
}

// FindAsset returns the download URL for the current platform's archive.
func FindAsset(release *Release, goos, goarch string) string {
	expected := AssetName(release.TagName, goos, goarch)
	for _, asset := range release.Assets {
		if asset.Name == expected {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

// FetchChecksums downloads and parses checksums.txt for the given release tag.
// Returns a map of filename → hex SHA256 digest.
func FetchChecksums(ctx context.Context, tagName string) (map[string]string, error) {
	url := fmt.Sprintf("https://github.com/madnh/tsk/releases/download/%s/checksums.txt", tagName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	result := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			hash, filename := parts[0], parts[1]
			result[filename] = hash
		}
	}
	return result, scanner.Err()
}

// VerifyChecksum computes the SHA256 of data and compares to the expected hex string.
func VerifyChecksum(data []byte, expected string) error {
	hash := sha256.Sum256(data)
	computed := hex.EncodeToString(hash[:])
	if computed != expected {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", computed, expected)
	}
	return nil
}

// DownloadAsset fetches the archive bytes from url, with progress callback.
func DownloadAsset(ctx context.Context, url string, progress func(downloaded, total int64)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	if progress != nil {
		// Wrap with progress tracking reader
		pr := &progressReader{reader: resp.Body, total: total, callback: progress}
		return io.ReadAll(pr)
	}
	return io.ReadAll(resp.Body)
}

// progressReader wraps io.Reader to track download progress.
type progressReader struct {
	reader   io.Reader
	total    int64
	read     int64
	callback func(int64, int64)
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	pr.read += int64(n)
	if pr.callback != nil {
		pr.callback(pr.read, pr.total)
	}
	return
}

// ExtractBinary extracts the binary from archive bytes (supports tar.gz and zip).
func ExtractBinary(archiveData []byte, archiveName string) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".tar.gz") {
		return extractFromTarGz(archiveData)
	} else if strings.HasSuffix(archiveName, ".zip") {
		return extractFromZip(archiveData)
	}
	return nil, fmt.Errorf("unsupported archive format: %s", archiveName)
}

func extractFromTarGz(data []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}

		// Look for binary named "tsk" or "tsk.exe"
		if strings.HasSuffix(header.Name, "tsk") || strings.HasSuffix(header.Name, "tsk.exe") {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary not found in archive")
}

func extractFromZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}

	for _, file := range zr.File {
		if strings.HasSuffix(file.Name, "tsk") || strings.HasSuffix(file.Name, "tsk.exe") {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("open file: %w", err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("binary not found in archive")
}

// ReplaceBinary atomically replaces the file at targetPath with newBinary.
func ReplaceBinary(targetPath string, newBinary []byte) error {
	// Write to temp file in same directory
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)
	tmpFile := filepath.Join(dir, base+".new")

	if err := os.WriteFile(tmpFile, newBinary, 0755); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// On Windows, handle atomic replacement differently
	if runtime.GOOS == "windows" {
		oldFile := targetPath + ".old"
		os.Remove(oldFile) // best-effort
		if err := os.Rename(targetPath, oldFile); err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("backup old binary: %w", err)
		}
		if err := os.Rename(tmpFile, targetPath); err != nil {
			// Try to restore from backup
			os.Rename(oldFile, targetPath)
			return fmt.Errorf("move new binary: %w", err)
		}
		os.Remove(oldFile) // best-effort cleanup
	} else {
		// Unix: atomic rename
		if err := os.Rename(tmpFile, targetPath); err != nil {
			return fmt.Errorf("replace binary: %w", err)
		}
	}

	return nil
}

// SelfPath returns the absolute path of the currently running binary.
func SelfPath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable: %w", err)
	}
	// Resolve symlinks
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}
	return path, nil
}
