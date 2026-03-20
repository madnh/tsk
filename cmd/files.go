package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/output"
)

var filesCmd = &cobra.Command{
	Use:   "files <task-id>",
	Short: "Track modified files for a task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		task, err := taskStore.Read(id)
		if err != nil {
			output.Fail(fmt.Sprintf("Error reading task: %v", err))
		}
		if task == nil {
			output.Fail(fmt.Sprintf("Task not found: %s", id))
		}

		addFiles, _ := cmd.Flags().GetString("add")

		if addFiles != "" {
			newFiles := splitAndTrim(addFiles)
			// Merge unique
			seen := map[string]bool{}
			for _, f := range task.Files {
				seen[f] = true
			}
			for _, f := range newFiles {
				if !seen[f] {
					task.Files = append(task.Files, f)
					seen[f] = true
				}
			}
			taskStore.Write(task)

			output.Print(output.Result{
				Data: map[string]interface{}{
					"id":    task.ID,
					"files": task.Files,
					"added": newFiles,
				},
				Pretty: func() {
					fmt.Printf("\n  Files for %s%s%s:\n", output.Bold, output.ColorID(task.ID), output.Reset)
					for _, f := range task.Files {
						fmt.Printf("    %s\n", f)
					}
					fmt.Println()
				},
			})
		} else {
			output.Print(output.Result{
				Data: map[string]interface{}{
					"id":    task.ID,
					"files": task.Files,
				},
				Pretty: func() {
					if len(task.Files) == 0 {
						fmt.Printf("\n  No files for %s\n\n", output.ColorID(task.ID))
					} else {
						fmt.Printf("\n  Files for %s%s%s:\n", output.Bold, output.ColorID(task.ID), output.Reset)
						for _, f := range task.Files {
							fmt.Printf("    %s\n", f)
						}
						fmt.Println()
					}
				},
			})
		}
	},
}

func init() {
	filesCmd.Flags().String("add", "", "Comma-separated files to add")
	rootCmd.AddCommand(filesCmd)
}

