package engine

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/store"
)

// LoopEngine handles loop state machine transitions
type LoopEngine struct {
	LoopStore  *store.LoopStore
	TaskStore  *store.TaskStore
	PhaseStore *store.PhaseStore
}

// AdvanceResult holds the outcome of a state machine advance
type AdvanceResult struct {
	Status    string `json:"status"`
	Step      string `json:"step"`
	Task      string `json:"task"`
	Phase     string `json:"phase"`
	Iteration int    `json:"iteration"`
	Action    string `json:"action"`
	Duration  string `json:"duration,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Advance processes the current step result and transitions the state machine
func (e *LoopEngine) Advance(state *model.LoopState, resume bool) (*AdvanceResult, error) {
	// Handle resume from paused
	if resume {
		if state.Status != model.LoopPaused {
			return nil, fmt.Errorf("loop is not paused")
		}
		if !e.LoopStore.FileExists("human-input.md") {
			return nil, fmt.Errorf("no human-input.md found. Write guidance there first")
		}
		state.Status = model.LoopRunning
		e.LoopStore.WriteState(state)
		task := state.Task
		if task == "" {
			task = "none"
		}
		e.LoopStore.Log(fmt.Sprintf("RESUME step=%s task=%s (human-input provided)", state.Step, task))
		return &AdvanceResult{
			Status: model.LoopRunning,
			Step:   state.Step,
			Task:   state.Task,
			Phase:  state.Phase,
			Action: "Resumed with human input",
		}, nil
	}

	if state.Status != model.LoopRunning {
		reason := "Waiting for human input"
		if state.Status == model.LoopComplete {
			reason = "All phases done"
		}
		return &AdvanceResult{
			Status: state.Status,
			Step:   state.Step,
			Task:   state.Task,
			Phase:  state.Phase,
			Reason: reason,
		}, nil
	}

	result := e.LoopStore.ReadFile("step-result.txt")
	allTasks, _ := e.TaskStore.All()

	nextStatus := model.LoopRunning
	reason := ""
	action := ""

	switch state.Step {
	case model.StepAnalyze:
		e.LoopStore.DeleteFile("step-result.txt")

		if result == "ALL_TASKS_DONE" {
			nextStatus = model.LoopComplete
			reason = fmt.Sprintf("All tasks in phase %s are done. Add more tasks or mark phase done manually.", state.Phase)
			action = fmt.Sprintf("Phase %s: all tasks done. Waiting for user.", state.Phase)
		} else if result == "HAS_TASKS" {
			action = e.pickNextTask(state, allTasks)
			if state.Status == model.LoopPaused {
				nextStatus = model.LoopPaused
				reason = "All tasks are blocked or done, but phase is not complete"
			}
		} else {
			return nil, fmt.Errorf("unknown analyze result: %q. Expected HAS_TASKS or ALL_TASKS_DONE", result)
		}

	case model.StepImplement:
		if result == "BLOCKED" {
			e.LoopStore.DeleteFile("step-result.txt")
			nextStatus = model.LoopPaused
			reason = "AI is blocked. See tasks/loop/feedback.md for details."
			action = "Paused — blocked. Write guidance to tasks/loop/human-input.md"
		} else {
			e.LoopStore.DeleteFile("step-result.txt")
			state.Step = model.StepReview
			state.TotalIterations++
			action = "Implementation done. Moving to review."
		}

	case model.StepReview:
		e.LoopStore.DeleteFile("step-result.txt")

		if result == "SHIP" {
			// Validate AC checkboxes
			if state.Task != "" {
				task, _ := e.TaskStore.Read(state.Task)
				if task != nil {
					acRe := regexp.MustCompile(`## Acceptance Criteria\n([\s\S]*?)(?:\n## |\s*$)`)
					acMatch := acRe.FindStringSubmatch(task.Body)
					if len(acMatch) > 1 {
						acText := acMatch[1]
						unchecked := strings.Count(acText, "- [ ]")
						checked := strings.Count(strings.ToLower(acText), "- [x]")
						if unchecked > 0 && checked == 0 {
							// Force REVISE
							state.Iteration++
							state.Step = model.StepImplement
							e.LoopStore.WriteFile("feedback.md",
								fmt.Sprintf("Review said SHIP but %d acceptance criteria are still unchecked.\nYou must verify and check off each criterion in the task file before shipping.\n", unchecked))
							e.LoopStore.WriteState(state)
							e.LoopStore.Log(fmt.Sprintf("END result=SHIP_REJECTED ac_unchecked=%d → forcing REVISE", unchecked))
							return &AdvanceResult{
								Status:    model.LoopRunning,
								Step:      model.StepImplement,
								Task:      state.Task,
								Phase:     state.Phase,
								Iteration: state.Iteration,
								Action:    fmt.Sprintf("SHIP rejected — %d AC unchecked. Revising.", unchecked),
							}, nil
						}
					}

					// Mark task done
					if task.Status == "in_progress" {
						task.Status = "review"
						e.TaskStore.Write(task)
					}
					if task.Status == "review" || task.Status == "in_progress" {
						task.Status = "done"
						task.Completed = today()
						e.TaskStore.Write(task)
					}
				}
			}

			e.LoopStore.DeleteFile("feedback.md")
			e.LoopStore.DeleteFile("work-summary.md")
			e.LoopStore.DeleteFile("human-input.md")

			if state.LockTask {
				nextStatus = model.LoopComplete
				reason = "Locked task completed"
				action = fmt.Sprintf("Task %s shipped! (locked task done)", state.Task)
			} else {
				state.Task = ""
				state.Step = model.StepAnalyze
				state.Iteration = 0
				action = "Task shipped! Re-analyzing phase."
			}
		} else if result == "REVISE" {
			state.Iteration++
			if state.Iteration >= state.MaxIterations {
				nextStatus = model.LoopPaused
				reason = fmt.Sprintf("Max iterations (%d) reached for task %s", state.MaxIterations, state.Task)
				action = "Paused — max iterations reached. Write guidance to tasks/loop/human-input.md"
			} else {
				state.Step = model.StepImplement
				action = fmt.Sprintf("Revising (iteration %d/%d)", state.Iteration+1, state.MaxIterations)
			}
		} else {
			return nil, fmt.Errorf("unknown review result: %q. Expected SHIP or REVISE", result)
		}

	default:
		return nil, fmt.Errorf("unknown step: %s", state.Step)
	}

	// Compute duration
	duration := ""
	if state.StepStartedAt != "" {
		if t, err := time.Parse(time.RFC3339, state.StepStartedAt); err == nil {
			elapsed := int(math.Round(time.Since(t).Seconds()))
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
	e.LoopStore.WriteState(state)

	// Log
	logResult := result
	if logResult == "" {
		logResult = "auto"
	}
	logEntry := fmt.Sprintf("END result=%s → %s", logResult, action)
	if duration != "" {
		logEntry += fmt.Sprintf(" (%s)", duration)
	}
	if reason != "" {
		logEntry += fmt.Sprintf(" reason=%q", reason)
	}
	e.LoopStore.Log(logEntry)

	return &AdvanceResult{
		Status:    nextStatus,
		Step:      state.Step,
		Task:      state.Task,
		Phase:     state.Phase,
		Iteration: state.Iteration,
		Action:    action,
		Duration:  duration,
		Reason:    reason,
	}, nil
}

func (e *LoopEngine) pickNextTask(state *model.LoopState, allTasks []*model.Task) string {
	var phaseTasks []*model.Task
	for _, t := range allTasks {
		if t.Phase == state.Phase {
			phaseTasks = append(phaseTasks, t)
		}
	}

	if state.LockTask {
		task, _ := e.TaskStore.Read(state.Task)
		if task == nil {
			state.Status = model.LoopPaused
			return fmt.Sprintf("Locked task %s not found", state.Task)
		}
		if task.Status == "done" {
			state.Status = model.LoopComplete
			return fmt.Sprintf("Task %s is done.", state.Task)
		}
		if task.Status == "pending" {
			task.Status = "in_progress"
			task.Started = today()
			e.TaskStore.Write(task)
		}
		state.Step = model.StepImplement
		state.Iteration = 0
		return fmt.Sprintf("Starting locked task %s", state.Task)
	}

	// Find candidates: prefer in_progress, then available pending
	type candidate struct {
		task     *model.Task
		resuming bool
	}
	var candidates []candidate

	for _, t := range phaseTasks {
		if t.Status == "in_progress" {
			candidates = append(candidates, candidate{t, true})
		}
	}
	for _, t := range phaseTasks {
		if t.Status == "pending" && !IsBlocked(t, allTasks) {
			candidates = append(candidates, candidate{t, false})
		}
	}

	if len(candidates) > 0 {
		// Sort: priority first, then prefer in_progress
		for i := 0; i < len(candidates); i++ {
			for j := i + 1; j < len(candidates); j++ {
				pi := PriorityVal(candidates[i].task.Priority)
				pj := PriorityVal(candidates[j].task.Priority)
				if pj < pi || (pj == pi && candidates[j].resuming && !candidates[i].resuming) {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}

		picked := candidates[0]
		if picked.resuming {
			state.Task = picked.task.ID
			state.Step = model.StepImplement
			state.Iteration = 0
			return fmt.Sprintf("Resuming in-progress task %s (%s)", picked.task.ID, picked.task.Priority)
		}

		picked.task.Status = "in_progress"
		picked.task.Started = today()
		e.TaskStore.Write(picked.task)
		state.Task = picked.task.ID
		state.Step = model.StepImplement
		state.Iteration = 0
		return fmt.Sprintf("Started task %s: %s (%s)", picked.task.ID, picked.task.Title, picked.task.Priority)
	}

	// Check for tasks in review
	for _, t := range phaseTasks {
		if t.Status == "review" {
			state.Task = t.ID
			state.Step = model.StepReview
			state.Iteration = 0
			return fmt.Sprintf("Reviewing task %s", t.ID)
		}
	}

	state.Status = model.LoopPaused
	return "Paused — no actionable tasks"
}

func today() string {
	return time.Now().Format("2006-01-02")
}
