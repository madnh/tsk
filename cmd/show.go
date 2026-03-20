package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/output"
)

var showCmd = &cobra.Command{
	Use:   "show <task-id>",
	Short: "Show task details",
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
		blocked := engine.IsBlocked(task, allTasks)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"id":        task.ID,
				"title":     task.Title,
				"status":    task.Status,
				"phase":     task.Phase,
				"feature":   task.Feature,
				"priority":  task.Priority,
				"depends":   task.Depends,
				"spec":      task.Spec,
				"files":     task.Files,
				"created":   task.Created,
				"started":   task.Started,
				"completed": task.Completed,
				"blocked":   blocked,
				"body":      strings.TrimSpace(task.Body),
			},
			Pretty: func() {
				fmt.Printf("\n%s%s: %s%s%s\n\n",
					output.Bold, output.ColorID(task.ID), output.Bold, task.Title, output.Reset)
				fmt.Printf("  %sStatus:%s   %s\n", output.Dim, output.Reset, output.ColorStatus(task.Status))
				fmt.Printf("  %sPhase:%s    %s\n", output.Dim, output.Reset, orDash(task.Phase))
				fmt.Printf("  %sFeature:%s  %s%s%s\n", output.Dim, output.Reset, output.Magenta, orDash(task.Feature), output.Reset)
				fmt.Printf("  %sPriority:%s %s\n", output.Dim, output.Reset, output.ColorPriority(orDefault(task.Priority, "medium")))

				if len(task.Depends) > 0 {
					var depParts []string
					for _, d := range task.Depends {
						depTask := findInTasks(allTasks, d)
						if depTask != nil {
							color := output.StatusColors[depTask.Status]
							depParts = append(depParts, fmt.Sprintf("%s %s(%s)%s", output.ColorID(d), color, depTask.Status, output.Reset))
						} else {
							depParts = append(depParts, fmt.Sprintf("%s %s(not found)%s", output.ColorID(d), output.Gray, output.Reset))
						}
					}
					fmt.Printf("  %sDepends:%s  %s\n", output.Dim, output.Reset, strings.Join(depParts, ", "))
				}
				if task.Spec != "" {
					fmt.Printf("  %sSpec:%s     %s\n", output.Dim, output.Reset, task.Spec)
				}
				if len(task.Files) > 0 {
					fmt.Printf("  %sFiles:%s    %s\n", output.Dim, output.Reset, strings.Join(task.Files, ", "))
				}
				if task.Created != "" {
					fmt.Printf("  %sCreated:%s  %s\n", output.Dim, output.Reset, task.Created)
				}
				if task.Started != "" {
					fmt.Printf("  %sStarted:%s  %s\n", output.Dim, output.Reset, task.Started)
				}
				if task.Completed != "" {
					fmt.Printf("  %sDone:%s     %s\n", output.Dim, output.Reset, task.Completed)
				}
				fmt.Printf("\n%s\n", output.RenderMarkdown(strings.TrimSpace(task.Body)))
			},
		})
	},
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func init() {
	rootCmd.AddCommand(showCmd)
}
