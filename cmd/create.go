package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/user/tsk/internal/engine"
	"github.com/user/tsk/internal/model"
	"github.com/user/tsk/internal/output"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new task",
	Run: func(cmd *cobra.Command, args []string) {
		title, _ := cmd.Flags().GetString("title")
		phase, _ := cmd.Flags().GetString("phase")
		feature, _ := cmd.Flags().GetString("feature")
		priority, _ := cmd.Flags().GetString("priority")
		depends, _ := cmd.Flags().GetString("depends")
		spec, _ := cmd.Flags().GetString("spec")
		useStdin, _ := cmd.Flags().GetBool("stdin")

		if title == "" {
			output.Fail("--title is required")
		}
		if phase == "" {
			output.Fail("--phase is required")
		}
		if feature == "" {
			output.Fail("--feature is required")
		}

		id, err := taskStore.NextID()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to get next ID: %v", err))
		}

		var body string
		if useStdin {
			data, _ := io.ReadAll(os.Stdin)
			input := string(data)
			if strings.TrimSpace(input) == "" {
				output.Fail("No input received from stdin")
			}
			body = "\n" + strings.TrimRight(input, "\n") + "\n\n## Log\n\n"
		} else {
			body = "\n## Description\n\nTODO: Add description\n\n## Acceptance Criteria\n\n- [ ] TODO: Define acceptance criteria\n\n## Log\n\n"
		}

		errors := engine.ValidateBody(body)
		if len(errors) > 0 {
			output.Fail(strings.Join(errors, ". "))
		}

		var depsList []string
		if depends != "" {
			depsList = splitAndTrim(depends)
			allTasks, _ := taskStore.All()
			for _, d := range depsList {
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
		}
		if depsList == nil {
			depsList = []string{}
		}

		if priority == "" {
			priority = "medium"
		}
		if !model.IsValidPriority(priority) {
			output.Fail(fmt.Sprintf("Invalid priority: %s. Valid: %s", priority, strings.Join(model.ValidPriorities, ", ")))
		}

		task := &model.Task{
			ID:        id,
			Title:     title,
			Status:    "pending",
			Phase:     phase,
			Feature:   feature,
			Priority:  priority,
			Depends:   depsList,
			Spec:      spec,
			Files:     []string{},
			Created:   todayStr(),
			Started:   "",
			Completed: "",
			Body:      body,
			FilePath:  filepath.Join(cfg.ItemsDir, id+".md"),
		}

		if err := taskStore.Write(task); err != nil {
			output.Fail(fmt.Sprintf("Failed to write task: %v", err))
		}

		relPath, _ := filepath.Rel(cfg.Root, task.FilePath)
		output.Print(output.Result{
			Data: map[string]interface{}{
				"created": id,
				"title":   title,
				"file":    relPath,
			},
			Pretty: func() {
				fmt.Printf("\n  %s%s%s Created %s%s%s: %s\n",
					output.StatusColors["pending"], output.StatusIcons["pending"], output.Reset,
					output.Bold, output.ColorID(id), output.Reset, title)
				fmt.Printf("  %sFile: %s%s\n\n", output.Dim, relPath, output.Reset)
			},
		})
	},
}

func init() {
	createCmd.Flags().String("title", "", "Task title (required)")
	createCmd.Flags().String("phase", "", "Phase number (required)")
	createCmd.Flags().String("feature", "", "Feature name (required)")
	createCmd.Flags().String("priority", "", "Priority (critical|high|medium|low)")
	createCmd.Flags().String("depends", "", "Comma-separated dependency task IDs")
	createCmd.Flags().String("spec", "", "Path to spec file")
	createCmd.Flags().Bool("stdin", false, "Read body from stdin")
	rootCmd.AddCommand(createCmd)
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func todayStr() string {
	return time.Now().Format("2006-01-02")
}
