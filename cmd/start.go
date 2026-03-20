package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/tsk/internal/engine"
	"github.com/user/tsk/internal/output"
)

var startCmd = &cobra.Command{
	Use:   "start <task-id>",
	Short: "Start a task (pending → in_progress)",
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
		if task.Status != "pending" {
			output.Fail(fmt.Sprintf("Cannot start: %s is '%s'. Only 'pending' tasks can be started.", id, task.Status))
		}

		allTasks, _ := taskStore.All()
		if engine.IsBlocked(task, allTasks) {
			pending := engine.GetPendingDeps(task, allTasks)
			output.Fail(fmt.Sprintf("Task %s is blocked by: %s", id, strings.Join(pending, ", ")))
		}

		task.Status = "in_progress"
		task.Started = todayStr()
		taskStore.Write(task)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"started": task.ID,
				"title":   task.Title,
				"spec":    task.Spec,
			},
			Pretty: func() {
				fmt.Printf("\n  %s%s%s Started %s%s%s: %s\n",
					output.StatusColors["in_progress"], output.StatusIcons["in_progress"], output.Reset,
					output.Bold, output.ColorID(task.ID), output.Reset, task.Title)
				if task.Spec != "" {
					fmt.Printf("  %sSpec:%s %s\n", output.Dim, output.Reset, task.Spec)
				}
				fmt.Println()
			},
		})
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
