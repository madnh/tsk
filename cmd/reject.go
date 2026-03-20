package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/tsk/internal/output"
)

var rejectCmd = &cobra.Command{
	Use:   "reject <task-id>",
	Short: "Reject a task (review → in_progress)",
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
		if task.Status != "review" {
			output.Fail(fmt.Sprintf("Cannot reject: %s is '%s'. Only 'review' tasks can be rejected.", id, task.Status))
		}

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		if useStdin {
			data, _ := io.ReadAll(os.Stdin)
			message = strings.TrimRight(string(data), "\n")
			if message == "" {
				output.Fail("No input received from stdin")
			}
		}
		if message == "" {
			output.Fail("Provide --message '...' or --stdin for rejection reason")
		}

		task.Status = "in_progress"
		task.Body += fmt.Sprintf("\n### %s - Developer\nRejected: %s\n", todayStr(), message)
		taskStore.Write(task)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"rejected": task.ID,
				"title":    task.Title,
				"reason":   message,
			},
			Pretty: func() {
				fmt.Printf("\n  %s↩%s Rejected %s%s%s: %s\n\n",
					output.Red, output.Reset,
					output.Bold, output.ColorID(task.ID), output.Reset, message)
			},
		})
	},
}

func init() {
	rejectCmd.Flags().String("message", "", "Rejection reason")
	rejectCmd.Flags().Bool("stdin", false, "Read message from stdin")
	rootCmd.AddCommand(rejectCmd)
}
