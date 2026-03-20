package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/output"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <task-id>",
	Short: "Delete a task",
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

		allTasks, _ := taskStore.All()
		dependents := engine.GetReverseDeps(id, allTasks)
		if len(dependents) > 0 {
			var ids []string
			for _, d := range dependents {
				ids = append(ids, d.ID)
			}
			output.Fail(fmt.Sprintf("Cannot delete: %s depend on %s", strings.Join(ids, ", "), id))
		}

		taskStore.Delete(id)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"deleted": id,
			},
			Pretty: func() {
				fmt.Printf("\n  Deleted %s\n\n", id)
			},
		})
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
