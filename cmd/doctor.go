package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/output"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check environment and project setup",
	Run: func(cmd *cobra.Command, args []string) {
		type check struct {
			Name   string `json:"name"`
			Status string `json:"status"` // ok, warning, error
			Detail string `json:"detail"`
		}

		var checks []check

		// tsk.yml
		if _, err := os.Stat(cfg.Root + "/tsk.yml"); err == nil {
			checks = append(checks, check{"tsk.yml", "ok", "Found at " + cfg.Root + "/tsk.yml"})
		} else {
			checks = append(checks, check{"tsk.yml", "error", "Not found. Run 'tsk init' to create."})
		}

		// tasks directories
		dirs := map[string]string{
			"tasks/items/":  cfg.ItemsDir,
			"tasks/phases/": cfg.PhasesDir,
			"tasks/loop/":   cfg.LoopDir,
		}
		for name, dir := range dirs {
			if _, err := os.Stat(dir); err == nil {
				checks = append(checks, check{name, "ok", "Exists"})
			} else {
				checks = append(checks, check{name, "warning", "Missing"})
			}
		}

		// git
		if _, err := exec.LookPath("git"); err == nil {
			checks = append(checks, check{"git", "ok", "Available"})
		} else {
			checks = append(checks, check{"git", "error", "Not found on PATH"})
		}

		// git worktree support
		_, wErr := exec.Command("git", "worktree", "list").CombinedOutput()
		if wErr == nil {
			checks = append(checks, check{"git worktree", "ok", "Supported"})
		} else {
			checks = append(checks, check{"git worktree", "warning", "May not be supported"})
		}

		// claude
		if _, err := exec.LookPath("claude"); err == nil {
			checks = append(checks, check{"claude", "ok", "Available"})
		} else {
			checks = append(checks, check{"claude", "warning", "Not found on PATH (needed for ralph)"})
		}

		// Root detection
		rootMethod := "current directory"
		if rootDir != "" {
			rootMethod = "--root-dir flag"
		} else if os.Getenv("TSK_ROOT_DIR") != "" {
			rootMethod = "TSK_ROOT_DIR env var"
		} else if _, err := os.Stat(cfg.Root + "/tsk.yml"); err == nil {
			rootMethod = "tsk.yml walk-up"
		} else if _, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
			rootMethod = "git root"
		}

		checks = append(checks, check{"root detection", "ok", fmt.Sprintf("%s → %s", rootMethod, cfg.Root)})

		// Env vars
		tskRoot := os.Getenv("TSK_ROOT_DIR")
		if tskRoot != "" {
			checks = append(checks, check{"TSK_ROOT_DIR", "ok", tskRoot})
		} else {
			checks = append(checks, check{"TSK_ROOT_DIR", "ok", "(not set)"})
		}

		output.Print(output.Result{
			Data: map[string]interface{}{"checks": checks},
			Pretty: func() {
				fmt.Printf("\n  %s%stsk doctor%s\n\n", output.Bold, output.Cyan, output.Reset)
				for _, c := range checks {
					var icon, color string
					switch c.Status {
					case "ok":
						icon = "✓"
						color = output.Green
					case "warning":
						icon = "!"
						color = output.Yellow
					case "error":
						icon = "✗"
						color = output.Red
					}
					fmt.Printf("  %s%s%s %-20s %s%s%s\n",
						color, icon, output.Reset, c.Name, output.Dim, c.Detail, output.Reset)
				}
				fmt.Println()
			},
		})
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
