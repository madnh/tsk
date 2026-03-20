package model

// Valid phase statuses
var ValidPhaseStatuses = []string{"pending", "defining", "ready", "in_progress", "done"}

// Phase represents a project phase
type Phase struct {
	Num         string            `json:"num"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Body        string            `json:"-"`
	FilePath    string            `json:"-"`
	RawMeta     map[string]string `json:"-"` // preserve unknown fields
}

// IsValidPhaseStatus checks if a phase status is valid
func IsValidPhaseStatus(s string) bool {
	for _, v := range ValidPhaseStatuses {
		if v == s {
			return true
		}
	}
	return false
}
