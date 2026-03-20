package model

// LoopState represents the state of the autonomous execution loop
type LoopState struct {
	Phase           string `json:"phase"`
	Task            string `json:"task"`
	Step            string `json:"step"`
	Iteration       int    `json:"iteration"`
	MaxIterations   int    `json:"maxIterations"`
	TotalIterations int    `json:"totalIterations"`
	Status          string `json:"status"`
	StartedAt       string `json:"startedAt"`
	LockTask        bool   `json:"lockTask"`
	StepStartedAt   string `json:"stepStartedAt,omitempty"`
}

// Loop steps
const (
	StepAnalyze   = "analyze"
	StepImplement = "implement"
	StepReview    = "review"
)

// Loop statuses
const (
	LoopRunning  = "running"
	LoopPaused   = "paused"
	LoopComplete = "complete"
)
