package engine

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/store"
)

// WorkerEngine handles worker state machine transitions for a single task
type WorkerEngine struct {
	WorkerStore *store.WorkerStore
	TaskStore   *store.TaskStore
}

// WorkerAdvanceResult holds the outcome of a worker state machine advance
type WorkerAdvanceResult struct {
	Status    string `json:"status"`
	Step      string `json:"step"`
	Iteration int    `json:"iteration"`
	Action    string `json:"action"`
	Duration  string `json:"duration,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Advance processes the current step result and transitions the state machine
func (e *WorkerEngine) Advance(state *model.WorkerState) (*WorkerAdvanceResult, error) {
	if state.Status != "running" {
		return nil, fmt.Errorf("worker is not running (status: %s)", state.Status)
	}

	step := state.CurrentStep()
	if step == "" {
		return nil, fmt.Errorf("no current step")
	}

	result := e.WorkerStore.ReadFile("step-result.txt")
	action := ""
	nextStatus := "running"
	reason := ""

	switch step {
	case "analyze":
		e.WorkerStore.DeleteFile("step-result.txt")

		if result == "ALL_TASKS_DONE" {
			// Task analysis complete, all work done (unusual for single task)
			state.Status = "done"
			action = "Analysis complete — no work needed"
		} else if result == "HAS_TASKS" {
			// Move to next step
			state.StepIndex++
			if state.StepIndex >= len(state.Workflow) {
				state.Status = "done"
				action = "Workflow complete"
			} else {
				action = fmt.Sprintf("Advancing to step: %s", state.CurrentStep())
			}
		} else if result == "BLOCKED" {
			nextStatus = "blocked"
			reason = "AI is blocked on analysis"
			action = "Blocked — see feedback.md for details"
		} else if result == "" {
			// No result = success for analyze
			state.StepIndex++
			if state.StepIndex >= len(state.Workflow) {
				state.Status = "done"
				action = "Workflow complete"
			} else {
				action = fmt.Sprintf("Advancing to step: %s", state.CurrentStep())
			}
		} else {
			return nil, fmt.Errorf("unknown analyze result: %q", result)
		}

	case "implement", "write", "brainstorm", "spec", "test":
		e.WorkerStore.DeleteFile("step-result.txt")

		if result == "BLOCKED" {
			nextStatus = "blocked"
			reason = fmt.Sprintf("AI is blocked on %s step", step)
			action = fmt.Sprintf("Blocked on %s — see feedback.md for details", step)
		} else {
			// No result or empty = success
			state.StepIndex++
			if state.StepIndex >= len(state.Workflow) {
				state.Status = "done"
				action = "Workflow complete"
			} else {
				action = fmt.Sprintf("Step %s done. Advancing to: %s", step, state.CurrentStep())
			}
		}

	case "review":
		e.WorkerStore.DeleteFile("step-result.txt")

		if result == "SHIP" {
			// Validate AC checkboxes if this is an implementation task
			task, _ := e.TaskStore.Read(state.TaskID)
			if task != nil && task.Body != "" {
				acRe := regexp.MustCompile(`## Acceptance Criteria\n([\s\S]*?)(?:\n## |\s*$)`)
				acMatch := acRe.FindStringSubmatch(task.Body)
				if len(acMatch) > 1 {
					acText := acMatch[1]
					unchecked := strings.Count(acText, "- [ ]")
					checked := strings.Count(strings.ToLower(acText), "- [x]")
					if unchecked > 0 && checked == 0 {
						// Force REVISE if ACs are unchecked
						state.Iteration++
						if state.Iteration >= state.MaxIterations {
							nextStatus = "blocked"
							reason = fmt.Sprintf("Max iterations (%d) reached and ACs still unchecked", state.MaxIterations)
							action = "Max iterations reached — blocked"
						} else {
							// Go back to previous step
							state.StepIndex--
							action = fmt.Sprintf("SHIP rejected — %d AC unchecked. Revising (iteration %d/%d)", unchecked, state.Iteration+1, state.MaxIterations)
						}
						e.WorkerStore.WriteFile("feedback.md",
							fmt.Sprintf("Review said SHIP but %d acceptance criteria are still unchecked.\nYou must verify and check off each criterion in the task file before shipping.\n", unchecked))
						e.WorkerStore.WriteState(state)
						e.WorkerStore.Log(fmt.Sprintf("SHIP_REJECTED ac_unchecked=%d → forcing REVISE", unchecked))
						return &WorkerAdvanceResult{
							Status:    nextStatus,
							Step:      state.CurrentStep(),
							Iteration: state.Iteration,
							Action:    action,
						}, nil
					}
				}

				// Mark task done
				if task.Status == "in_progress" {
					task.Status = "done"
					task.Completed = today()
					e.TaskStore.Write(task)
				}
			}

			e.WorkerStore.DeleteFile("feedback.md")
			e.WorkerStore.DeleteFile("work-summary.md")
			e.WorkerStore.DeleteFile("human-input.md")

			state.Status = "done"
			action = "Task shipped!"

		} else if result == "REVISE" {
			state.Iteration++
			if state.Iteration >= state.MaxIterations {
				nextStatus = "blocked"
				reason = fmt.Sprintf("Max iterations (%d) reached", state.MaxIterations)
				action = "Blocked — max iterations reached"
			} else {
				// Go back to previous step (usually implement)
				if state.StepIndex > 0 {
					state.StepIndex--
				}
				action = fmt.Sprintf("Revising (iteration %d/%d)", state.Iteration+1, state.MaxIterations)
			}
		} else if result == "" {
			// No result for review = success (auto advance)
			state.StepIndex++
			if state.StepIndex >= len(state.Workflow) {
				state.Status = "done"
				action = "Workflow complete"
			} else {
				action = fmt.Sprintf("Review done. Advancing to: %s", state.CurrentStep())
			}
		} else {
			return nil, fmt.Errorf("unknown review result: %q", result)
		}

	default:
		return nil, fmt.Errorf("unknown step: %s", step)
	}

	// Compute duration
	duration := ""
	if state.StepStartedAt != "" {
		if t, err := time.Parse(time.RFC3339, state.StepStartedAt); err == nil {
			elapsed := int(time.Since(t).Seconds())
			mins := elapsed / 60
			secs := elapsed % 60
			if mins > 0 {
				duration = fmt.Sprintf("%dm%ds", mins, secs)
			} else {
				duration = fmt.Sprintf("%ds", secs)
			}
		}
		state.StepStartedAt = ""
	}

	state.Status = nextStatus
	e.WorkerStore.WriteState(state)

	logEntry := fmt.Sprintf("END result=%q → %s", result, action)
	if duration != "" {
		logEntry += fmt.Sprintf(" (%s)", duration)
	}
	if reason != "" {
		logEntry += fmt.Sprintf(" reason=%q", reason)
	}
	e.WorkerStore.Log(logEntry)

	return &WorkerAdvanceResult{
		Status:    nextStatus,
		Step:      state.CurrentStep(),
		Iteration: state.Iteration,
		Action:    action,
		Duration:  duration,
		Reason:    reason,
	}, nil
}
