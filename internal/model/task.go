package model

// Valid task statuses
var ValidStatuses = []string{"pending", "in_progress", "review", "done"}

// Valid priorities
var ValidPriorities = []string{"critical", "high", "medium", "low"}

// Valid task types
var ValidTypes = []string{"feature", "bug", "docs", "refactor", "test", "chore"}

// PriorityOrder maps priority to sort order (lower = higher priority)
var PriorityOrder = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"low":      3,
}

// StatusTransitions defines valid status transitions
var StatusTransitions = map[string][]string{
	"pending":     {"in_progress"},
	"in_progress": {"review"},
	"review":      {"done", "in_progress"},
	"done":        {},
}

// Task represents a task with frontmatter metadata and body
type Task struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Phase     string   `json:"phase"`
	Feature   string   `json:"feature"`
	Priority  string   `json:"priority"`
	Type      string   `json:"type"`
	Depends   []string `json:"depends"`
	Spec      string   `json:"spec"`
	Files     []string `json:"files"`
	Created   string   `json:"created"`
	Started   string   `json:"started"`
	Completed string   `json:"completed"`
	Body      string   `json:"-"`
	FilePath  string   `json:"-"`
}

// MetaMap returns an ordered key-value slice for frontmatter serialization
func (t *Task) MetaMap() []KV {
	return []KV{
		{"id", t.ID},
		{"title", t.Title},
		{"status", t.Status},
		{"phase", t.Phase},
		{"feature", t.Feature},
		{"priority", t.Priority},
		{"type", t.Type},
		{"depends", t.Depends},
		{"spec", t.Spec},
		{"files", t.Files},
		{"created", t.Created},
		{"started", t.Started},
		{"completed", t.Completed},
	}
}

// KV is a key-value pair for ordered serialization
type KV struct {
	Key   string
	Value interface{}
}

// IsValidStatus checks if a status is valid
func IsValidStatus(s string) bool {
	for _, v := range ValidStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsValidPriority checks if a priority is valid
func IsValidPriority(p string) bool {
	for _, v := range ValidPriorities {
		if v == p {
			return true
		}
	}
	return false
}

// IsValidType checks if a task type is valid
func IsValidType(t string) bool {
	for _, v := range ValidTypes {
		if v == t {
			return true
		}
	}
	return false
}
