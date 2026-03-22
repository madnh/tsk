package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
)

var editCmd = &cobra.Command{
	Use:   "edit <task-id>",
	Short: "Edit task metadata",
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

		if s, _ := cmd.Flags().GetString("status"); s != "" {
			output.Fail("Cannot edit status directly. Use: start, done, approve, reject commands.")
		}

		if v, _ := cmd.Flags().GetString("title"); v != "" {
			task.Title = v
		}
		if v, _ := cmd.Flags().GetString("phase"); v != "" {
			task.Phase = v
		}
		if v, _ := cmd.Flags().GetString("feature"); v != "" {
			task.Feature = v
		}
		if v, _ := cmd.Flags().GetString("priority"); v != "" {
			if !model.IsValidPriority(v) {
				output.Fail(fmt.Sprintf("Invalid priority: %s. Valid: %s", v, strings.Join(model.ValidPriorities, ", ")))
			}
			task.Priority = v
		}
		if v, _ := cmd.Flags().GetString("type"); v != "" {
			if !model.IsValidType(v) {
				output.Fail(fmt.Sprintf("Invalid type: %s. Valid: %s", v, strings.Join(model.ValidTypes, ", ")))
			}
			task.Type = v
		}
		if v, _ := cmd.Flags().GetString("depends"); v != "" {
			newDeps := splitAndTrim(v)
			allTasks, _ := taskStore.All()
			for _, d := range newDeps {
				found := false
				for _, t := range allTasks {
					if t.ID == d {
						found = true
						break
					}
				}
				if !found {
					output.Fail(fmt.Sprintf("Dependency not found: %s", d))
				}
			}
			if engine.HasCircularDep(id, newDeps, allTasks) {
				output.Fail(fmt.Sprintf("Circular dependency detected. %s cannot depend on %s", id, strings.Join(newDeps, ", ")))
			}
			task.Depends = newDeps
		}
		if v, _ := cmd.Flags().GetString("spec"); v != "" {
			task.Spec = v
		}

		taskStore.Write(task)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"updated": task.ID,
			},
			Pretty: func() {
				fmt.Printf("\n  Updated %s%s%s\n\n", output.Bold, output.ColorID(task.ID), output.Reset)
			},
		})
	},
}

func init() {
	editCmd.Flags().String("title", "", "New title")
	editCmd.Flags().String("phase", "", "New phase")
	editCmd.Flags().String("feature", "", "New feature")
	editCmd.Flags().String("priority", "", "New priority")
	editCmd.Flags().String("type", "", "New type (feature|bug|docs|refactor|test|chore)")
	editCmd.Flags().String("depends", "", "New dependencies (comma-separated)")
	editCmd.Flags().String("spec", "", "New spec path")
	editCmd.Flags().String("status", "", "")
	editCmd.Flags().MarkHidden("status")
	rootCmd.AddCommand(editCmd)
}
