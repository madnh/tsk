package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/tsk/internal/output"
)

var logCmd = &cobra.Command{
	Use:   "log <task-id>",
	Short: "Add a log entry to a task",
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

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		author, _ := cmd.Flags().GetString("author")
		if author == "" {
			author = "AI Agent"
		}

		if useStdin {
			data, _ := io.ReadAll(os.Stdin)
			message = strings.TrimRight(string(data), "\n")
			if strings.TrimSpace(message) == "" {
				output.Fail("No input received from stdin")
			}
		}
		if message == "" {
			output.Fail("Provide --message '...' or --stdin")
		}

		date := todayStr()
		entry := fmt.Sprintf("\n### %s - %s\n%s\n", date, author, message)
		task.Body += entry
		taskStore.Write(task)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"logged":  task.ID,
				"date":    date,
				"author":  author,
				"message": message,
			},
			Pretty: func() {
				fmt.Printf("\n  Logged to %s%s%s:\n", output.Bold, output.ColorID(task.ID), output.Reset)
				fmt.Printf("  %s%s - %s%s\n", output.Dim, date, author, output.Reset)
				fmt.Printf("  %s\n\n", message)
			},
		})
	},
}

func init() {
	logCmd.Flags().String("message", "", "Log message")
	logCmd.Flags().Bool("stdin", false, "Read message from stdin")
	logCmd.Flags().String("author", "", "Author (default: AI Agent)")
	rootCmd.AddCommand(logCmd)
}
