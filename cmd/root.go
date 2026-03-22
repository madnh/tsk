package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/config"
	"github.com/madnh/tsk/internal/output"
	"github.com/madnh/tsk/internal/store"
	"github.com/madnh/tsk/internal/updater"
)

var (
	rootDir      string
	outputFormat string
	cfg          *config.Config
	taskStore    *store.TaskStore
	phaseStore   *store.PhaseStore
	loopStore    *store.LoopStore
	appVersion   string
	appCommit    string
	appDate      string
)

var rootCmd = &cobra.Command{
	Use:   "tsk",
	Short: "Task management CLI for structured project execution",
	Long:  "tsk — Manage implementation tasks, phases, and autonomous execution loops.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set output format
		if outputFormat == "json" {
			output.CurrentFormat = output.FormatJSON
		}

		// Skip config loading for top-level init, help, version, and update
		if cmd.Name() == "help" || cmd.Name() == "completion" || cmd.Name() == "version" || cmd.Name() == "update" {
			return
		}
		// Only skip for tsk init (not loop init)
		if cmd.Name() == "init" && cmd.Parent() != nil && cmd.Parent().Name() == "tsk" {
			return
		}

		cfg = config.Load(rootDir)
		taskStore = store.NewTaskStore(cfg.ItemsDir)
		phaseStore = store.NewPhaseStore(cfg.PhasesDir)
		loopStore = store.NewLoopStore(cfg.LoopDir)

		// Ensure directories exist
		os.MkdirAll(cfg.ItemsDir, 0755)
		os.MkdirAll(cfg.PhasesDir, 0755)

		// Check for updates in background if enabled (non-blocking)
		if config.GetUpdateCheckOnStartup() && output.CurrentFormat == output.FormatPretty {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(),
					time.Duration(config.GetUpdateTimeout())*time.Second)
				defer cancel()
				latest, err := updater.FetchLatestVersion(ctx)
				if err != nil || !updater.IsNewer(latest, appVersion) {
					return
				}
				fmt.Fprintf(os.Stderr, "\nUpdate available: %s → %s\nRun 'tsk update' to upgrade.\n\n", appVersion, latest)
			}()
		}
	},
}

// SetVersion sets the version info from build-time injected values
func SetVersion(v, c, d string) {
	appVersion = v
	appCommit = c
	appDate = d
}

func init() {
	rootCmd.PersistentFlags().StringVar(&rootDir, "root-dir", "", "Project root directory")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "pretty", "Output format (json|pretty)")
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
