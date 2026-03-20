package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/output"
)

var approveCmd = &cobra.Command{
	Use:   "approve <task-id>",
	Short: "Approve a task (review → done)",
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
			output.Fail(fmt.Sprintf("Cannot approve: %s is '%s'. Only 'review' tasks can be approved.", id, task.Status))
		}

		task.Status = "done"

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		if useStdin {
			data, _ := io.ReadAll(os.Stdin)
			message = strings.TrimRight(string(data), "\n")
		}

		if message != "" {
			task.Body += fmt.Sprintf("\n### %s - Developer\nApproved. %s\n", todayStr(), message)
		} else {
			task.Body += fmt.Sprintf("\n### %s - Developer\nApproved.\n", todayStr())
		}
		taskStore.Write(task)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"approved": task.ID,
				"title":    task.Title,
			},
			Pretty: func() {
				fmt.Printf("\n  %s%s%s Approved %s%s%s: %s\n\n",
					output.StatusColors["done"], output.StatusIcons["done"], output.Reset,
					output.Bold, output.ColorID(task.ID), output.Reset, task.Title)
			},
		})
	},
}

func init() {
	approveCmd.Flags().String("message", "", "Approval message")
	approveCmd.Flags().Bool("stdin", false, "Read message from stdin")
	rootCmd.AddCommand(approveCmd)
}
