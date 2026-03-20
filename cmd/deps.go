package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/user/tsk/internal/engine"
	"github.com/user/tsk/internal/model"
	"github.com/user/tsk/internal/output"
)

var depsCmd = &cobra.Command{
	Use:   "deps <task-id>",
	Short: "Show dependency tree",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		reverse, _ := cmd.Flags().GetBool("reverse")

		allTasks, _ := taskStore.All()
		task := findInTasks(allTasks, id)
		if task == nil {
			output.Fail(fmt.Sprintf("Task not found: %s", id))
		}

		if reverse {
			rdeps := engine.GetReverseDeps(id, allTasks)
			type depInfo struct {
				ID     string `json:"id"`
				Title  string `json:"title"`
				Status string `json:"status"`
			}
			var depList []depInfo
			for _, t := range rdeps {
				depList = append(depList, depInfo{ID: t.ID, Title: t.Title, Status: t.Status})
			}

			output.Print(output.Result{
				Data: map[string]interface{}{
					"id":         id,
					"dependents": depList,
				},
				Pretty: func() {
					if len(rdeps) == 0 {
						fmt.Printf("\n  No tasks depend on %s%s%s\n\n", output.Bold, output.ColorID(id), output.Reset)
						return
					}
					fmt.Printf("\n  Tasks depending on %s%s%s:\n\n", output.Bold, output.ColorID(id), output.Reset)
					for _, t := range rdeps {
						color := output.StatusColors[t.Status]
						icon := output.StatusIcons[t.Status]
						if icon == "" {
							icon = "?"
						}
						fmt.Printf("    %s%s%s %s: %s %s(%s)%s\n",
							color, icon, output.Reset,
							output.ColorID(t.ID), t.Title, color, t.Status, output.Reset)
					}
					fmt.Println()
				},
			})
			return
		}

		tree := engine.GetDepTree(id, allTasks)

		type flatDep struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Status   string `json:"status"`
			Depth    int    `json:"depth"`
			Circular bool   `json:"circular,omitempty"`
		}
		var flat []flatDep
		var flatten func(node *engine.DepTreeNode, depth int)
		flatten = func(node *engine.DepTreeNode, depth int) {
			for _, child := range node.Children {
				flat = append(flat, flatDep{
					ID: child.ID, Title: child.Title, Status: child.Status,
					Depth: depth, Circular: child.Circular,
				})
				if !child.Circular && !child.Missing {
					flatten(child, depth+1)
				}
			}
		}
		flatten(tree, 0)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"id":           id,
				"dependencies": flat,
			},
			Pretty: func() {
				if len(tree.Children) == 0 {
					fmt.Printf("\n  %s%s%s has no dependencies\n\n", output.Bold, output.ColorID(id), output.Reset)
					return
				}
				fmt.Printf("\n  Dependencies of %s%s%s:\n\n", output.Bold, output.ColorID(id), output.Reset)
				printDepTree(tree, "  ")
				fmt.Println()
			},
		})
	},
}

func printDepTree(node *engine.DepTreeNode, indent string) {
	for _, child := range node.Children {
		if child.Circular {
			fmt.Printf("%s%s↻ %s (circular!)%s\n", indent, output.Red, child.ID, output.Reset)
		} else if child.Missing {
			fmt.Printf("%s%s? %s (not found)%s\n", indent, output.Red, child.ID, output.Reset)
		} else {
			color := output.StatusColors[child.Status]
			icon := output.StatusIcons[child.Status]
			if child.Status == "done" {
				icon = "✓"
			}
			if icon == "" {
				icon = "?"
			}
			fmt.Printf("%s%s%s%s %s: %s %s(%s)%s\n",
				indent, color, icon, output.Reset,
				output.ColorID(child.ID), child.Title, color, child.Status, output.Reset)
			if len(child.Children) > 0 {
				printDepTree(child, indent+"  ")
			}
		}
	}
}

func findInTasks(tasks []*model.Task, id string) *model.Task {
	for _, t := range tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func init() {
	depsCmd.Flags().Bool("reverse", false, "Show tasks that depend on this task")
	rootCmd.AddCommand(depsCmd)
}
