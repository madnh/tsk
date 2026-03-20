package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/output"
)

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Suggest next available task by priority",
	Run: func(cmd *cobra.Command, args []string) {
		allTasks, err := taskStore.All()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read tasks: %v", err))
		}

		task := engine.NextAvailable(allTasks)
		if task == nil {
			output.Print(output.Result{
				Data: map[string]interface{}{
					"next":    nil,
					"message": "No available tasks",
				},
				Pretty: func() {
					fmt.Print("\n  No available tasks.\n\n")
				},
			})
			return
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"next": map[string]interface{}{
					"id":       task.ID,
					"title":    task.Title,
					"feature":  task.Feature,
					"phase":    task.Phase,
					"priority": task.Priority,
					"spec":     task.Spec,
					"depends":  task.Depends,
				},
			},
			Pretty: func() {
				fmt.Printf("\n  %sNext: %s%s\n", output.Bold, output.ColorID(task.ID), output.Reset)
				fmt.Printf("  %sTitle:%s    %s\n", output.Dim, output.Reset, task.Title)
				fmt.Printf("  %sFeature:%s  %s%s%s\n", output.Dim, output.Reset, output.Magenta, orDash(task.Feature), output.Reset)
				fmt.Printf("  %sPhase:%s    %s\n", output.Dim, output.Reset, orDash(task.Phase))
				fmt.Printf("  %sPriority:%s %s\n", output.Dim, output.Reset, output.ColorPriority(orDefault(task.Priority, "medium")))
				if task.Spec != "" {
					fmt.Printf("  %sSpec:%s     %s\n", output.Dim, output.Reset, task.Spec)
				}
				if len(task.Depends) > 0 {
					fmt.Printf("  %sDepends:%s  %s %s(all done ✓)%s\n",
						output.Dim, output.Reset,
						strings.Join(task.Depends, ", "),
						output.Green, output.Reset)
				}
				fmt.Println()
			},
		})
	},
}

func init() {
	rootCmd.AddCommand(nextCmd)
}
