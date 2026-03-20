package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
)

var progressCmd = &cobra.Command{
	Use:   "progress",
	Short: "Show progress per phase",
	Run: func(cmd *cobra.Command, args []string) {
		tasks, _ := taskStore.All()
		phases, _ := phaseStore.All()

		totalDone := countTasks(tasks, func(t *model.Task) bool { return t.Status == "done" })

		type phaseProgress struct {
			Phase      string `json:"phase"`
			Name       string `json:"name"`
			Total      int    `json:"total"`
			Done       int    `json:"done"`
			InProgress int    `json:"in_progress"`
			Review     int    `json:"review"`
			Pending    int    `json:"pending"`
		}

		var phaseList []phaseProgress
		for _, p := range phases {
			pt := filterTasks(tasks, func(t *model.Task) bool { return t.Phase == p.Num })
			phaseList = append(phaseList, phaseProgress{
				Phase:      p.Num,
				Name:       p.Name,
				Total:      len(pt),
				Done:       countTasks(pt, func(t *model.Task) bool { return t.Status == "done" }),
				InProgress: countTasks(pt, func(t *model.Task) bool { return t.Status == "in_progress" }),
				Review:     countTasks(pt, func(t *model.Task) bool { return t.Status == "review" }),
				Pending:    countTasks(pt, func(t *model.Task) bool { return t.Status == "pending" }),
			})
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"overall": map[string]int{
					"total": len(tasks),
					"done":  totalDone,
				},
				"phases": phaseList,
			},
			Pretty: func() {
				fmt.Printf("\n%s%s  Progress%s\n\n", output.Bold, output.Cyan, output.Reset)
				fmt.Printf("  Overall: %s\n\n", output.ProgressBar(totalDone, len(tasks), 20))

				for _, p := range phaseList {
					fmt.Printf("  %sPhase %s: %s%s\n", output.Bold, p.Phase, p.Name, output.Reset)
					fmt.Printf("  %s\n", output.ProgressBar(p.Done, p.Total, 20))
					var parts []string
					if p.Done > 0 {
						parts = append(parts, fmt.Sprintf("%s%d done%s", output.Green, p.Done, output.Reset))
					}
					if p.InProgress > 0 {
						parts = append(parts, fmt.Sprintf("%s%d active%s", output.Yellow, p.InProgress, output.Reset))
					}
					if p.Review > 0 {
						parts = append(parts, fmt.Sprintf("%s%d review%s", output.Blue, p.Review, output.Reset))
					}
					if p.Pending > 0 {
						parts = append(parts, fmt.Sprintf("%s%d pending%s", output.Gray, p.Pending, output.Reset))
					}
					if len(parts) > 0 {
						fmt.Printf("  %s\n", strings.Join(parts, " · "))
					}
					fmt.Println()
				}
			},
		})
	},
}

func init() {
	rootCmd.AddCommand(progressCmd)
}
