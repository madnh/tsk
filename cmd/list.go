package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks with optional filters",
	Run: func(cmd *cobra.Command, args []string) {
		phase, _ := cmd.Flags().GetString("phase")
		status, _ := cmd.Flags().GetString("status")
		feature, _ := cmd.Flags().GetString("feature")
		available, _ := cmd.Flags().GetBool("available")

		allTasks, err := taskStore.All()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read tasks: %v", err))
		}

		tasks := allTasks
		if phase != "" {
			tasks = filterTasks(tasks, func(t *model.Task) bool { return t.Phase == phase })
		}
		if status != "" {
			tasks = filterTasks(tasks, func(t *model.Task) bool { return t.Status == status })
		}
		if feature != "" {
			tasks = filterTasks(tasks, func(t *model.Task) bool { return t.Feature == feature })
		}
		if available {
			tasks = filterTasks(tasks, func(t *model.Task) bool {
				return t.Status == "pending" && !engine.IsBlocked(t, allTasks)
			})
		}

		type taskInfo struct {
			ID       string   `json:"id"`
			Title    string   `json:"title"`
			Status   string   `json:"status"`
			Phase    string   `json:"phase"`
			Feature  string   `json:"feature"`
			Priority string   `json:"priority"`
			Depends  []string `json:"depends"`
			Spec     string   `json:"spec"`
			Blocked  bool     `json:"blocked"`
		}

		var taskList []taskInfo
		for _, t := range tasks {
			taskList = append(taskList, taskInfo{
				ID:       t.ID,
				Title:    t.Title,
				Status:   t.Status,
				Phase:    t.Phase,
				Feature:  t.Feature,
				Priority: t.Priority,
				Depends:  t.Depends,
				Spec:     t.Spec,
				Blocked:  t.Status == "pending" && engine.IsBlocked(t, allTasks),
			})
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"count": len(taskList),
				"tasks": taskList,
			},
			Pretty: func() {
				if len(tasks) == 0 {
					fmt.Println("\n  No tasks found.\n")
					return
				}
				fmt.Printf("\n  %s%d task(s)%s\n\n", output.Bold, len(tasks), output.Reset)
				fmt.Printf("  %s%-12s %-14s %-6s %-20s %-10s %-20s Title%s\n",
					output.Dim, "ID", "Status", "Phase", "Feature", "Priority", "Depends", output.Reset)
				fmt.Printf("  %s%s%s\n", output.Dim, strings.Repeat("─", 110), output.Reset)

				for _, t := range tasks {
					bl := t.Status == "pending" && engine.IsBlocked(t, allTasks)
					statusKey := t.Status
					if bl {
						statusKey = "blocked"
					}
					color := output.StatusColors[statusKey]
					icon := output.StatusIcons[statusKey]
					if icon == "" {
						icon = "?"
					}
					deps := "-"
					if len(t.Depends) > 0 {
						deps = strings.Join(t.Depends, ", ")
					}
					ph := orDash(t.Phase)
					feat := orDash(t.Feature)
					pri := t.Priority
					if pri == "" {
						pri = "medium"
					}
					fmt.Printf("  %s%s%s %-10s %s%-14s%s %-6s %s%-20s%s %-20s %s%-20s%s %s\n",
						color, icon, output.Reset,
						output.ColorID(t.ID),
						color, statusKey, output.Reset,
						ph,
						output.Magenta, feat, output.Reset,
						output.ColorPriority(pri),
						output.Dim, deps, output.Reset,
						t.Title)
				}
				fmt.Println()
			},
		})
	},
}

func filterTasks(tasks []*model.Task, fn func(*model.Task) bool) []*model.Task {
	var result []*model.Task
	for _, t := range tasks {
		if fn(t) {
			result = append(result, t)
		}
	}
	return result
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func init() {
	listCmd.Flags().String("phase", "", "Filter by phase")
	listCmd.Flags().String("status", "", "Filter by status")
	listCmd.Flags().String("feature", "", "Filter by feature")
	listCmd.Flags().Bool("available", false, "Show only available (pending, not blocked) tasks")
	rootCmd.AddCommand(listCmd)
}
