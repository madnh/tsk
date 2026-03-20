package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/output"
)

var doneCmd = &cobra.Command{
	Use:   "done <task-id>",
	Short: "Mark task as done (in_progress → review)",
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
		if task.Status != "in_progress" {
			output.Fail(fmt.Sprintf("Cannot mark done: %s is '%s'. Only 'in_progress' tasks can be marked done.", id, task.Status))
		}

		task.Status = "review"
		task.Completed = todayStr()
		taskStore.Write(task)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"review": task.ID,
				"title":  task.Title,
			},
			Pretty: func() {
				fmt.Printf("\n  %s%s%s %s%s%s ready for review: %s\n\n",
					output.StatusColors["review"], output.StatusIcons["review"], output.Reset,
					output.Bold, output.ColorID(task.ID), output.Reset, task.Title)
			},
		})
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
