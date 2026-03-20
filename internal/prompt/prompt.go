package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"
	"github.com/user/tsk/internal/model"
	"github.com/user/tsk/internal/store"
)

// Generator creates prompts for loop steps
type Generator struct {
	LoopStore  *store.LoopStore
	TaskStore  *store.TaskStore
	PhaseStore *store.PhaseStore
	RootDir    string
}

// Generate creates a prompt for the current loop step
func (g *Generator) Generate(state *model.LoopState) (string, error) {
	phase, err := g.PhaseStore.Find(state.Phase)
	if err != nil || phase == nil {
		return "", fmt.Errorf("phase %s not found", state.Phase)
	}

	allTasks, _ := g.TaskStore.All()
	var phaseTasks []*model.Task
	for _, t := range allTasks {
		if t.Phase == state.Phase {
			phaseTasks = append(phaseTasks, t)
		}
	}

	var prompt string
	switch state.Step {
	case model.StepAnalyze:
		prompt = g.analyzePrompt(phase, phaseTasks, allTasks)
	case model.StepImplement:
		prompt, err = g.implementPrompt(state, phase, allTasks)
		if err != nil {
			return "", err
		}
	case model.StepReview:
		prompt, err = g.reviewPrompt(state)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown step: %s", state.Step)
	}

	// Append custom prompts
	prompt = g.appendCustomPrompts(prompt, state.Step)

	return prompt, nil
}

func (g *Generator) analyzePrompt(phase *model.Phase, phaseTasks []*model.Task, allTasks []*model.Task) string {
	taskListStr := "  (no tasks)"
	if len(phaseTasks) > 0 {
		var lines []string
		for _, t := range phaseTasks {
			blocked := ""
			if t.Status == "pending" && isBlocked(t, allTasks) {
				blocked = " [BLOCKED]"
			}
			lines = append(lines, fmt.Sprintf("  - %s: %s (%s)%s", t.ID, t.Title, t.Status, blocked))
		}
		taskListStr = strings.Join(lines, "\n")
	}

	specsList := g.collectSpecs()

	body := strings.TrimSpace(phase.Body)

	return fmt.Sprintf(`You are analyzing Phase %s: %s.
Description: %s
Body:
%s

Current tasks in this phase:
%s

Available feature specs:
%s

Instructions:
1. Assess the current state of this phase
2. Check if all tasks are done, if there are available tasks to work on, or if new tasks need to be created

Write ONE of these results to tasks/loop/step-result.txt:
- "HAS_TASKS" — if there are available (pending, not blocked) tasks to work on
- "ALL_TASKS_DONE" — if all tasks in this phase are done (or there are no tasks)

IMPORTANT: Write ONLY the result keyword to tasks/loop/step-result.txt (no extra text).
IMPORTANT: Do NOT decide if the phase is complete. Do NOT create new tasks. Only check existing task statuses.
Do NOT implement any code. Only analyze and write the result file.`,
		phase.Num, phase.Name, phase.Description, body, taskListStr, specsList)
}

func (g *Generator) implementPrompt(state *model.LoopState, phase *model.Phase, allTasks []*model.Task) (string, error) {
	if state.Task == "" {
		return "", fmt.Errorf("no task assigned for implement step")
	}
	task, err := g.TaskStore.Read(state.Task)
	if err != nil || task == nil {
		return "", fmt.Errorf("task %s not found", state.Task)
	}

	specFile := task.Spec
	acMatch := regexp.MustCompile(`## Acceptance Criteria\n([\s\S]*?)(?:\n## |\s*$)`).FindStringSubmatch(task.Body)
	ac := "(none found in task body)"
	if len(acMatch) > 1 {
		ac = strings.TrimSpace(acMatch[1])
	}

	feedback := g.LoopStore.ReadFile("feedback.md")
	humanInput := g.LoopStore.ReadFile("human-input.md")

	feedbackSection := "## Previous Feedback\nFirst iteration — no feedback yet."
	if feedback != "" {
		feedbackSection = fmt.Sprintf("## Previous Feedback\n%s", feedback)
	}

	humanSection := ""
	if humanInput != "" {
		humanSection = fmt.Sprintf("\n## Human Guidance\n%s", humanInput)
	}

	specInstr := "Review the task description above"
	if specFile != "" {
		specInstr = fmt.Sprintf("Read the spec file: %s", specFile)
	}
	feedbackInstr := "Plan your implementation approach"
	if feedback != "" {
		feedbackInstr = "Address the feedback from previous review"
	}
	humanInstr := ""
	if humanInput != "" {
		humanInstr = "\n3. Follow the human guidance provided above"
	}

	specSection := "No spec file linked."
	if specFile != "" {
		specSection = fmt.Sprintf("Read: %s", specFile)
	}

	tskCmd := "tsk"

	prompt := fmt.Sprintf(`You are implementing %s: %s
Iteration %d/%d for this task.

## Spec
%s

## Acceptance Criteria
%s

%s
%s
## Instructions
1. %s
2. %s%s
4. Implement code with tests
5. Run: `+"`go test ./... -count=1`"+` and `+"`go vet ./...`"+`
6. Track files: `+"`%s files %s --add \"file1,file2\" -o json`"+`
7. Log progress: `+"`%s log %s --stdin -o json << 'EOF'\n...\nEOF`"+`
8. Commit your changes using conventional commits with the task ID as scope:
   `+"```bash"+`
   git add <files you changed>
   git commit -m "feat(%s): <short description>"
   `+"```"+`
   - Use `+"`feat`"+` for new features, `+"`fix`"+` for bug fixes, `+"`refactor`"+` for refactoring
   - You may make multiple commits for logically separate changes
   - Do NOT commit task state files (tasks/loop/*)
9. Write a summary of what you did to tasks/loop/work-summary.md
10. If blocked on something you cannot resolve:
    - Write "BLOCKED" to tasks/loop/step-result.txt
    - Write your questions/blockers to tasks/loop/feedback.md

IMPORTANT: When done implementing, do NOT write to step-result.txt — the loop will advance automatically.
IMPORTANT: Always commit your code changes before finishing. Uncommitted work is invisible to the next iteration.
State persists through FILES ONLY. You have no memory of previous iterations.`,
		task.ID, task.Title,
		state.Iteration+1, state.MaxIterations,
		specSection, ac,
		feedbackSection, humanSection,
		specInstr, feedbackInstr, humanInstr,
		tskCmd, task.ID,
		tskCmd, task.ID,
		task.ID)

	return prompt, nil
}

func (g *Generator) reviewPrompt(state *model.LoopState) (string, error) {
	if state.Task == "" {
		return "", fmt.Errorf("no task assigned for review step")
	}
	task, err := g.TaskStore.Read(state.Task)
	if err != nil || task == nil {
		return "", fmt.Errorf("task %s not found", state.Task)
	}

	acMatch := regexp.MustCompile(`## Acceptance Criteria\n([\s\S]*?)(?:\n## |\s*$)`).FindStringSubmatch(task.Body)
	ac := "(none found in task body)"
	if len(acMatch) > 1 {
		ac = strings.TrimSpace(acMatch[1])
	}

	filesList := "(none tracked)"
	if len(task.Files) > 0 {
		filesList = strings.Join(task.Files, ", ")
	}

	workSummary := g.LoopStore.ReadFile("work-summary.md")
	if workSummary == "" {
		workSummary = "(no summary provided)"
	}

	return fmt.Sprintf(`You are reviewing %s: %s

Task file: tasks/items/%s.md

## Acceptance Criteria
%s

## Modified Files
%s

## Work Summary
%s

## Instructions
1. Read the modified files listed above
2. Run: `+"`go test ./... -count=1`"+`
3. Run: `+"`go vet ./...`"+`
4. Check that changes are committed:
   Run `+"`git log --oneline -10`"+` and verify there are commits with "%s" in the message.
   If code changes exist but are NOT committed, this is a REVISE — feedback: "commit your changes".
5. For EACH acceptance criterion, verify against actual code:
   - If met: check it off in the task file (change `+"`- [ ]`"+` to `+"`- [x]`"+`)
   - If NOT met: leave unchecked and note what's missing

6. After checking all criteria, update the task file tasks/items/%s.md:
   Replace each verified `+"`- [ ]`"+` with `+"`- [x]`"+` in the Acceptance Criteria section.

7. Decide the result:
   If ALL criteria are checked AND tests pass AND changes are committed:
     Write "SHIP" to tasks/loop/step-result.txt
   If any criterion is NOT met OR changes are uncommitted:
     Write "REVISE" to tasks/loop/step-result.txt
     Write specific, actionable feedback to tasks/loop/feedback.md
     Be precise: which criterion failed, what's missing, what needs to change.

IMPORTANT: Write ONLY "SHIP" or "REVISE" to tasks/loop/step-result.txt (no extra text).
IMPORTANT: You MUST update the AC checkboxes in the task file before writing the result.`,
		task.ID, task.Title,
		task.ID, ac, filesList, workSummary,
		task.ID, task.ID), nil
}

func (g *Generator) appendCustomPrompts(prompt, step string) string {
	// Read prompts from tsk.yml: ralph.prompt.all / ralph.prompt.<step>
	allPrompt := stripCommentLines(viper.GetString("ralph.prompt.all"))
	stepPrompt := stripCommentLines(viper.GetString("ralph.prompt." + step))

	if allPrompt != "" {
		prompt += fmt.Sprintf("\n\n## Additional Instructions\n%s", allPrompt)
	}
	if stepPrompt != "" {
		prompt += fmt.Sprintf("\n\n## Additional Instructions (%s)\n%s", step, stepPrompt)
	}

	return prompt
}

// stripCommentLines removes lines starting with # and returns trimmed content.
// This allows tsk.yml prompts to have comment-only defaults that produce no output.
func stripCommentLines(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func (g *Generator) collectSpecs() string {
	specsDir := filepath.Join(g.RootDir, "docs", "features")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return ""
	}
	var lines []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		specPath := filepath.Join(specsDir, e.Name(), "spec.md")
		if _, err := os.Stat(specPath); err == nil {
			lines = append(lines, fmt.Sprintf("  - docs/features/%s/spec.md", e.Name()))
		}
	}
	return strings.Join(lines, "\n")
}

func isBlocked(task *model.Task, allTasks []*model.Task) bool {
	if len(task.Depends) == 0 {
		return false
	}
	taskMap := make(map[string]*model.Task, len(allTasks))
	for _, t := range allTasks {
		taskMap[t.ID] = t
	}
	for _, depID := range task.Depends {
		dep, ok := taskMap[depID]
		if !ok || dep.Status != "done" {
			return true
		}
	}
	return false
}
