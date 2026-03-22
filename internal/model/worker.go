package model

import "time"

// WorkerState represents the state of a task being executed by a worker
type WorkerState struct {
	TaskID        string    `json:"taskId"`
	TaskType      string    `json:"taskType"`
	Workflow      []string  `json:"workflow"`      // ordered steps, e.g. ["implement", "review"]
	StepIndex     int       `json:"stepIndex"`     // current position in Workflow
	Iteration     int       `json:"iteration"`     // current revision iteration
	MaxIterations int       `json:"maxIterations"`
	Status        string    `json:"status"`        // running, blocked, done, failed
	WorktreePath  string    `json:"worktreePath"`  // e.g. worktrees/TASK-001
	BranchName    string    `json:"branchName"`    // e.g. task/TASK-001
	StartedAt     string    `json:"startedAt"`     // RFC3339
	StepStartedAt string    `json:"stepStartedAt,omitempty"`
	BlockedReason string    `json:"blockedReason,omitempty"`
	PID           int       `json:"pid,omitempty"`
}

// CurrentStep returns the step name for the current index
func (w *WorkerState) CurrentStep() string {
	if w.StepIndex < 0 || w.StepIndex >= len(w.Workflow) {
		return ""
	}
	return w.Workflow[w.StepIndex]
}

// IsLastStep returns true if this is the final step in the workflow
func (w *WorkerState) IsLastStep() bool {
	return w.StepIndex == len(w.Workflow)-1
}

// SupervisorState represents the state of the supervisor managing multiple workers
type SupervisorState struct {
	Phase     string         `json:"phase"`
	Status    string         `json:"status"`    // running, complete, paused
	StartedAt string         `json:"startedAt"` // RFC3339
	Workers   []WorkerEntry  `json:"workers"`
}

// WorkerEntry is a lightweight record of an active/recent worker
type WorkerEntry struct {
	TaskID    string `json:"taskId"`
	PID       int    `json:"pid"`
	Status    string `json:"status"`    // running, blocked, done, failed
	SpawnedAt string `json:"spawnedAt"` // RFC3339
}

// NewWorkerState creates a new WorkerState for a task
func NewWorkerState(taskID, taskType string, workflow []string, maxIterations int) *WorkerState {
	return &WorkerState{
		TaskID:        taskID,
		TaskType:      taskType,
		Workflow:      workflow,
		StepIndex:     0,
		Iteration:     0,
		MaxIterations: maxIterations,
		Status:        "running",
		StartedAt:     time.Now().Format(time.RFC3339),
	}
}

// NewSupervisorState creates a new SupervisorState
func NewSupervisorState(phase string) *SupervisorState {
	return &SupervisorState{
		Phase:     phase,
		Status:    "running",
		StartedAt: time.Now().Format(time.RFC3339),
		Workers:   []WorkerEntry{},
	}
}
