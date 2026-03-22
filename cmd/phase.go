package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/config"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
)

var phaseCmd = &cobra.Command{
	Use:   "phase [number]",
	Short: "List phases or show/update a specific phase",
	Run: func(cmd *cobra.Command, args []string) {
		phases, _ := phaseStore.All()
		tasks, _ := taskStore.All()

		if len(args) == 0 {
			// List all phases
			type phaseInfo struct {
				Phase       string `json:"phase"`
				Name        string `json:"name"`
				Status      string `json:"status"`
				Description string `json:"description"`
				Tasks       int    `json:"tasks"`
				Done        int    `json:"done"`
			}
			var list []phaseInfo
			for _, p := range phases {
				pt := filterTasks(tasks, func(t *model.Task) bool { return t.Phase == p.Num })
				pd := countTasks(pt, func(t *model.Task) bool { return t.Status == "done" })
				list = append(list, phaseInfo{
					Phase: p.Num, Name: p.Name, Status: p.Status,
					Description: p.Description, Tasks: len(pt), Done: pd,
				})
			}
			output.Print(output.Result{
				Data: map[string]interface{}{"phases": list},
				Pretty: func() {
					fmt.Printf("\n%s%sPhases%s\n\n", output.Bold, output.Cyan, output.Reset)
					for _, p := range phases {
						pt := filterTasks(tasks, func(t *model.Task) bool { return t.Phase == p.Num })
						pd := countTasks(pt, func(t *model.Task) bool { return t.Status == "done" })
						fmt.Printf("  %s %sPhase %s: %s%s\n",
							output.ColorPhaseStatus(p.Status), output.Bold, p.Num, p.Name, output.Reset)
						fmt.Printf("    %s%s%s\n", output.Dim, p.Description, output.Reset)
						if len(pt) > 0 {
							fmt.Printf("    %s\n", output.ProgressBar(pd, len(pt), 20))
						}
						fmt.Println()
					}
				},
			})
			return
		}

		// Show or update specific phase
		num := args[0]
		var phase *model.Phase
		for _, p := range phases {
			if p.Num == num {
				phase = p
				break
			}
		}
		if phase == nil {
			output.Fail(fmt.Sprintf("Phase not found: %s", num))
		}

		status, _ := cmd.Flags().GetString("status")
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")

		if status != "" || name != "" || description != "" {
			// Update
			if status != "" {
				if !model.IsValidPhaseStatus(status) {
					output.Fail(fmt.Sprintf("Invalid status: %s. Valid: %s", status, strings.Join(model.ValidPhaseStatuses, ", ")))
				}
				phase.Status = status
			}
			if name != "" {
				phase.Name = name
			}
			if description != "" {
				phase.Description = description
			}
			phaseStore.Write(phase)

			var updated []string
			if status != "" {
				updated = append(updated, "status → "+status)
			}
			if name != "" {
				updated = append(updated, "name → "+name)
			}
			if description != "" {
				updated = append(updated, "description → "+description)
			}

			output.Print(output.Result{
				Data: map[string]interface{}{
					"updated":     fmt.Sprintf("phase-%s", phase.Num),
					"status":      phase.Status,
					"name":        phase.Name,
					"description": phase.Description,
				},
				Pretty: func() {
					fmt.Printf("\n  Phase %s updated: %s\n\n", phase.Num, strings.Join(updated, ", "))
				},
			})
			return
		}

		// Show detail
		phaseTasks := filterTasks(tasks, func(t *model.Task) bool { return t.Phase == phase.Num })
		type taskBrief struct {
			ID      string   `json:"id"`
			Title   string   `json:"title"`
			Status  string   `json:"status"`
			Depends []string `json:"depends"`
		}
		var taskList []taskBrief
		for _, t := range phaseTasks {
			taskList = append(taskList, taskBrief{ID: t.ID, Title: t.Title, Status: t.Status, Depends: t.Depends})
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"phase":       phase.Num,
				"name":        phase.Name,
				"status":      phase.Status,
				"description": phase.Description,
				"body":        strings.TrimSpace(phase.Body),
				"tasks":       taskList,
			},
			Pretty: func() {
				fmt.Printf("\n%s%sPhase %s: %s%s %s\n\n",
					output.Bold, output.Cyan, phase.Num, phase.Name, output.Reset,
					output.ColorPhaseStatus(phase.Status))
				fmt.Printf("  %s%s%s\n\n", output.Dim, phase.Description, output.Reset)
				if body := strings.TrimSpace(phase.Body); body != "" {
					fmt.Printf("%s\n\n", output.RenderMarkdown(body))
				}
				if len(phaseTasks) > 0 {
					fmt.Printf("%s  Tasks:%s\n", output.Bold, output.Reset)
					for _, t := range phaseTasks {
						bl := t.Status == "pending" && engine.IsBlocked(t, tasks)
						statusKey := t.Status
						if bl {
							statusKey = "blocked"
						}
						color := output.StatusColors[statusKey]
						icon := output.StatusIcons[statusKey]
						if icon == "" {
							icon = "?"
						}
						depStr := ""
						if len(t.Depends) > 0 {
							depStr = fmt.Sprintf(" %s← %s%s", output.Dim, strings.Join(t.Depends, ", "), output.Reset)
						}
						fmt.Printf("  %s%s%s %s: %s %s(%s)%s%s\n",
							color, icon, output.Reset,
							output.ColorID(t.ID), t.Title, color, statusKey, output.Reset, depStr)
					}
				} else {
					fmt.Println("  No tasks yet.")
				}
				fmt.Println()
			},
		})
	},
}

var phaseLogCmd = &cobra.Command{
	Use:   "log <phase-num>",
	Short: "Add log entry to phase",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		num := args[0]
		phase, err := phaseStore.Find(num)
		if err != nil || phase == nil {
			output.Fail(fmt.Sprintf("Phase not found: %s", num))
		}

		message, _ := cmd.Flags().GetString("message")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		author, _ := cmd.Flags().GetString("author")
		if author == "" {
			author = "AI Agent"
		}

		if useStdin {
			data, _ := io.ReadAll(os.Stdin)
			message = strings.TrimRight(string(data), "\n")
			if strings.TrimSpace(message) == "" {
				output.Fail("No input received from stdin")
			}
		}
		if message == "" {
			output.Fail("Provide --message '...' or --stdin")
		}

		date := todayStr()
		entry := fmt.Sprintf("\n### %s - %s\n%s\n", date, author, message)
		phase.Body += entry
		phaseStore.Write(phase)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"logged":  fmt.Sprintf("phase-%s", phase.Num),
				"date":    date,
				"author":  author,
				"message": message,
			},
			Pretty: func() {
				fmt.Printf("\n  Logged to %sPhase %s%s:\n", output.Bold, phase.Num, output.Reset)
				fmt.Printf("  %s%s - %s%s\n", output.Dim, date, author, output.Reset)
				fmt.Printf("  %s\n\n", message)
			},
		})
	},
}

var phaseUpdateBodyCmd = &cobra.Command{
	Use:   "update-body <phase-num>",
	Short: "Replace phase body content",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		num := args[0]
		phase, err := phaseStore.Find(num)
		if err != nil || phase == nil {
			output.Fail(fmt.Sprintf("Phase not found: %s", num))
		}

		useStdin, _ := cmd.Flags().GetBool("stdin")
		if !useStdin {
			output.Fail("--stdin is required for update-body")
		}
		data, _ := io.ReadAll(os.Stdin)
		input := string(data)
		if strings.TrimSpace(input) == "" {
			output.Fail("No input received from stdin")
		}

		// Preserve log entries
		logIdx := strings.Index(phase.Body, "### ")
		logSection := ""
		if logIdx != -1 {
			logSection = "\n" + phase.Body[logIdx:]
		}

		phase.Body = "\n" + strings.TrimRight(input, "\n") + "\n" + logSection
		phaseStore.Write(phase)

		output.Print(output.Result{
			Data: map[string]interface{}{
				"updated": fmt.Sprintf("phase-%s", phase.Num),
			},
			Pretty: func() {
				fmt.Printf("\n  Updated body of %sPhase %s%s\n\n", output.Bold, phase.Num, output.Reset)
			},
		})
	},
}

var phaseCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new phase",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		status, _ := cmd.Flags().GetString("status")

		if name == "" {
			output.Fail("--name is required")
		}

		if !model.IsValidPhaseStatus(status) {
			output.Fail(fmt.Sprintf("Invalid status: %s. Valid: %s", status, strings.Join(model.ValidPhaseStatuses, ", ")))
		}

		num, err := phaseStore.NextNum()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to get next phase number: %v", err))
		}

		phase := &model.Phase{
			Num:         num,
			Name:        name,
			Description: description,
			Status:      status,
			Body:        "\n",
			FilePath:    fmt.Sprintf("%s/phase-%s.md", cfg.PhasesDir, num),
			RawMeta:     make(map[string]string),
		}

		if err := phaseStore.Write(phase); err != nil {
			output.Fail(fmt.Sprintf("Failed to write phase: %v", err))
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"created":     fmt.Sprintf("phase-%s", num),
				"num":         num,
				"name":        name,
				"description": description,
				"status":      status,
			},
			Pretty: func() {
				fmt.Printf("\n  Created %sPhase %s: %s%s (%s)\n\n",
					output.Bold, num, name, output.Reset, status)
			},
		})
	},
}

var phaseDeleteCmd = &cobra.Command{
	Use:   "delete <num>",
	Short: "Delete a phase",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		num := args[0]

		phase, err := phaseStore.Find(num)
		if err != nil || phase == nil {
			output.Fail(fmt.Sprintf("Phase not found: %s", num))
		}

		// Check if any tasks reference this phase
		tasks, _ := taskStore.All()
		var referencingTasks []string
		for _, t := range tasks {
			if t.Phase == num {
				referencingTasks = append(referencingTasks, t.ID)
			}
		}
		if len(referencingTasks) > 0 {
			output.Fail(fmt.Sprintf("Cannot delete phase %s: referenced by tasks %s", num, strings.Join(referencingTasks, ", ")))
		}

		if err := phaseStore.Delete(num); err != nil {
			output.Fail(fmt.Sprintf("Failed to delete phase: %v", err))
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"deleted": fmt.Sprintf("phase-%s", num),
			},
			Pretty: func() {
				fmt.Printf("\n  Deleted Phase %s: %s\n\n", num, phase.Name)
			},
		})
	},
}

var phaseSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync phases from tsk.yml config",
	Run: func(cmd *cobra.Command, args []string) {
		configPhases := config.GetPhases()
		if len(configPhases) == 0 {
			output.Fail("No phases defined in tsk.yml config")
		}

		var created, skipped []string
		for _, cp := range configPhases {
			if cp.Num == "" || cp.Name == "" {
				continue
			}
			existing, _ := phaseStore.Find(cp.Num)
			if existing != nil {
				skipped = append(skipped, cp.Num)
				continue
			}

			phase := &model.Phase{
				Num:         cp.Num,
				Name:        cp.Name,
				Description: cp.Description,
				Status:      "defining",
				Body:        "\n",
				FilePath:    fmt.Sprintf("%s/phase-%s.md", cfg.PhasesDir, cp.Num),
				RawMeta:     make(map[string]string),
			}
			if err := phaseStore.Write(phase); err != nil {
				output.Fail(fmt.Sprintf("Failed to write phase %s: %v", cp.Num, err))
			}
			created = append(created, cp.Num)
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"created": created,
				"skipped": skipped,
			},
			Pretty: func() {
				fmt.Printf("\n%s%sPhase Sync%s\n\n", output.Bold, output.Cyan, output.Reset)
				for _, num := range created {
					fmt.Printf("  %s+ Phase %s created%s\n", output.Green, num, output.Reset)
				}
				for _, num := range skipped {
					fmt.Printf("  %s~ Phase %s skipped (already exists)%s\n", output.Dim, num, output.Reset)
				}
				fmt.Println()
			},
		})
	},
}

func init() {
	phaseCmd.Flags().String("status", "", "Update phase status (pending|defining|ready|in_progress|done)")
	phaseCmd.Flags().String("name", "", "Update phase name")
	phaseCmd.Flags().String("description", "", "Update phase description")

	phaseCreateCmd.Flags().String("name", "", "Phase name (required)")
	phaseCreateCmd.Flags().String("description", "", "Phase description")
	phaseCreateCmd.Flags().String("status", "defining", "Phase status: pending|defining|ready|in_progress|done (default: defining)")

	phaseLogCmd.Flags().String("message", "", "Log message")
	phaseLogCmd.Flags().Bool("stdin", false, "Read message from stdin")
	phaseLogCmd.Flags().String("author", "", "Author (default: AI Agent)")

	phaseUpdateBodyCmd.Flags().Bool("stdin", false, "Read body from stdin")

	phaseCmd.AddCommand(phaseCreateCmd)
	phaseCmd.AddCommand(phaseDeleteCmd)
	phaseCmd.AddCommand(phaseSyncCmd)
	phaseCmd.AddCommand(phaseLogCmd)
	phaseCmd.AddCommand(phaseUpdateBodyCmd)
	rootCmd.AddCommand(phaseCmd)
}
