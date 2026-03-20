package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/config"
	"github.com/madnh/tsk/internal/output"
	"github.com/madnh/tsk/internal/store"
)

var (
	rootDir      string
	outputFormat string
	cfg          *config.Config
	taskStore    *store.TaskStore
	phaseStore   *store.PhaseStore
	loopStore    *store.LoopStore
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

		// Skip config loading for top-level init and help
		if cmd.Name() == "help" || cmd.Name() == "completion" {
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
	},
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
