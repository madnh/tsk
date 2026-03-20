package engine

import "github.com/madnh/tsk/internal/model"

// RunnablePhaseStatuses are phase statuses eligible for Ralph
var RunnablePhaseStatuses = map[string]bool{
	"ready":       true,
	"in_progress": true,
}

// IsPhaseRunnable checks if a phase can be run by Ralph
func IsPhaseRunnable(phase *model.Phase) bool {
	return RunnablePhaseStatuses[phase.Status]
}
