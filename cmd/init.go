package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/config"
	"github.com/madnh/tsk/internal/embedded"
	"github.com/madnh/tsk/internal/output"
)

var initForce bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize tsk project (create tsk.yml and directories)",
	Run: func(cmd *cobra.Command, args []string) {
		root := config.ResolveRoot(rootDir)
		configFile := filepath.Join(root, "tsk.yml")

		if _, err := os.Stat(configFile); err == nil && !initForce {
			output.Fail("tsk.yml already exists. Use --force to overwrite.")
		}

		// Write config file
		if err := os.WriteFile(configFile, embedded.DefaultConfig, 0644); err != nil {
			output.Fail(fmt.Sprintf("Failed to write tsk.yml: %v", err))
		}

		// Create directories
		dirs := []string{
			filepath.Join(root, "tasks", "items"),
			filepath.Join(root, "tasks", "phases"),
			filepath.Join(root, "tasks", "loop"),
		}
		for _, d := range dirs {
			os.MkdirAll(d, 0755)
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"initialized": true,
				"root":        root,
				"config":      "tsk.yml",
			},
			Pretty: func() {
				fmt.Printf("\n  Initialized tsk project at %s\n", root)
				fmt.Printf("  Created: tsk.yml, tasks/items/, tasks/phases/, tasks/loop/\n")
				fmt.Printf("  Edit tsk.yml to configure prompts: ralph.prompt.<step>\n\n")
			},
		})
	},
}

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing tsk.yml")
	rootCmd.AddCommand(initCmd)
}
