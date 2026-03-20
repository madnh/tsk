package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
)

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Dashboard overview of all tasks",
	Run: func(cmd *cobra.Command, args []string) {
		tasks, _ := taskStore.All()
		phases, _ := phaseStore.All()

		active := filterTasks(tasks, func(t *model.Task) bool { return t.Status == "in_progress" })
		review := filterTasks(tasks, func(t *model.Task) bool { return t.Status == "review" })
		blocked := filterTasks(tasks, func(t *model.Task) bool {
			return t.Status == "pending" && engine.IsBlocked(t, tasks)
		})
		done := filterTasks(tasks, func(t *model.Task) bool { return t.Status == "done" })

		type phaseInfo struct {
			Phase      string `json:"phase"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Total      int    `json:"total"`
			Done       int    `json:"done"`
			InProgress int    `json:"in_progress"`
			Review     int    `json:"review"`
			Pending    int    `json:"pending"`
			Blocked    int    `json:"blocked"`
		}

		var phaseList []phaseInfo
		for _, p := range phases {
			pt := filterTasks(tasks, func(t *model.Task) bool { return t.Phase == p.Num })
			pd := countTasks(pt, func(t *model.Task) bool { return t.Status == "done" })
			pip := countTasks(pt, func(t *model.Task) bool { return t.Status == "in_progress" })
			pr := countTasks(pt, func(t *model.Task) bool { return t.Status == "review" })
			pp := countTasks(pt, func(t *model.Task) bool { return t.Status == "pending" })
			pb := countTasks(pt, func(t *model.Task) bool {
				return t.Status == "pending" && engine.IsBlocked(t, tasks)
			})
			phaseList = append(phaseList, phaseInfo{
				Phase: p.Num, Name: p.Name, Status: p.Status,
				Total: len(pt), Done: pd, InProgress: pip, Review: pr,
				Pending: pp - pb, Blocked: pb,
			})
		}

		type taskBrief struct {
			ID      string   `json:"id"`
			Title   string   `json:"title"`
			Feature string   `json:"feature,omitempty"`
			Depends []string `json:"depends,omitempty"`
		}

		var activeList, reviewList, blockedList []taskBrief
		for _, t := range active {
			activeList = append(activeList, taskBrief{ID: t.ID, Title: t.Title, Feature: t.Feature})
		}
		for _, t := range review {
			reviewList = append(reviewList, taskBrief{ID: t.ID, Title: t.Title, Feature: t.Feature})
		}
		for _, t := range blocked {
			blockedList = append(blockedList, taskBrief{ID: t.ID, Title: t.Title, Depends: t.Depends})
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"summary": map[string]int{
					"total":   len(tasks),
					"done":    len(done),
					"active":  len(active),
					"review":  len(review),
					"blocked": len(blocked),
					"pending": len(tasks) - len(done) - len(active) - len(review) - len(blocked),
				},
				"phases":  phaseList,
				"active":  activeList,
				"review":  reviewList,
				"blocked": blockedList,
			},
			Pretty: func() {
				fmt.Printf("\n%s%sTask Board%s\n\n", output.Bold, output.Cyan, output.Reset)

				if len(phases) > 0 {
					for _, p := range phases {
						pt := filterTasks(tasks, func(t *model.Task) bool { return t.Phase == p.Num })
						pd := countTasks(pt, func(t *model.Task) bool { return t.Status == "done" })
						pip := countTasks(pt, func(t *model.Task) bool { return t.Status == "in_progress" })
						pr := countTasks(pt, func(t *model.Task) bool { return t.Status == "review" })
						pp := countTasks(pt, func(t *model.Task) bool { return t.Status == "pending" })
						pb := countTasks(pt, func(t *model.Task) bool {
							return t.Status == "pending" && engine.IsBlocked(t, tasks)
						})

						fmt.Printf("  %s %sPhase %s: %s%s\n",
							output.ColorPhaseStatus(p.Status), output.Bold, p.Num, p.Name, output.Reset)
						if len(pt) > 0 {
							fmt.Printf("    %s\n", output.ProgressBar(pd, len(pt), 20))
							var parts []string
							if pd > 0 {
								parts = append(parts, fmt.Sprintf("%s%d done%s", output.Green, pd, output.Reset))
							}
							if pip > 0 {
								parts = append(parts, fmt.Sprintf("%s%d active%s", output.Yellow, pip, output.Reset))
							}
							if pr > 0 {
								parts = append(parts, fmt.Sprintf("%s%d review%s", output.Blue, pr, output.Reset))
							}
							if pb > 0 {
								parts = append(parts, fmt.Sprintf("%s%d blocked%s", output.Red, pb, output.Reset))
							}
							if pp-pb > 0 {
								parts = append(parts, fmt.Sprintf("%s%d pending%s", output.Gray, pp-pb, output.Reset))
							}
							if len(parts) > 0 {
								fmt.Printf("    %s\n", strings.Join(parts, " · "))
							}
						}
						fmt.Println()
					}
				} else {
					fmt.Printf("  Overall: %s\n\n", output.ProgressBar(len(done), len(tasks), 20))
				}

				if len(active) > 0 {
					fmt.Printf("  %s%sActive:%s\n", output.Bold, output.Yellow, output.Reset)
					for _, t := range active {
						fmt.Printf("    %s%s%s %s  %s%s%s  %s\n",
							output.StatusColors["in_progress"], output.StatusIcons["in_progress"], output.Reset,
							output.ColorID(t.ID), output.Dim, t.Feature, output.Reset, t.Title)
					}
					fmt.Println()
				}
				if len(review) > 0 {
					fmt.Printf("  %s%sReview:%s\n", output.Bold, output.Blue, output.Reset)
					for _, t := range review {
						fmt.Printf("    %s%s%s %s  %s%s%s  %s\n",
							output.StatusColors["review"], output.StatusIcons["review"], output.Reset,
							output.ColorID(t.ID), output.Dim, t.Feature, output.Reset, t.Title)
					}
					fmt.Println()
				}
				if len(blocked) > 0 {
					fmt.Printf("  %s%sBlocked:%s\n", output.Bold, output.Red, output.Reset)
					for _, t := range blocked {
						deps := strings.Join(t.Depends, ", ")
						fmt.Printf("    %s%s%s %s  %s  %sblocked by %s%s\n",
							output.StatusColors["blocked"], output.StatusIcons["blocked"], output.Reset,
							output.ColorID(t.ID), t.Title, output.Dim, deps, output.Reset)
					}
					fmt.Println()
				}
			},
		})
	},
}

func countTasks(tasks []*model.Task, fn func(*model.Task) bool) int {
	n := 0
	for _, t := range tasks {
		if fn(t) {
			n++
		}
	}
	return n
}

func init() {
	rootCmd.AddCommand(boardCmd)
}
