package engine

import (
	"github.com/madnh/tsk/internal/model"
)

// IsBlocked checks if a task is blocked by unfinished dependencies
func IsBlocked(task *model.Task, allTasks []*model.Task) bool {
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

// GetPendingDeps returns dependency IDs that are not yet done
func GetPendingDeps(task *model.Task, allTasks []*model.Task) []string {
	taskMap := make(map[string]*model.Task, len(allTasks))
	for _, t := range allTasks {
		taskMap[t.ID] = t
	}
	var pending []string
	for _, depID := range task.Depends {
		dep, ok := taskMap[depID]
		if !ok || dep.Status != "done" {
			pending = append(pending, depID)
		}
	}
	return pending
}

// DepTreeNode represents a node in the dependency tree
type DepTreeNode struct {
	ID       string         `json:"id"`
	Title    string         `json:"title,omitempty"`
	Status   string         `json:"status,omitempty"`
	Circular bool           `json:"circular,omitempty"`
	Missing  bool           `json:"missing,omitempty"`
	Children []*DepTreeNode `json:"children,omitempty"`
}

// GetDepTree builds a dependency tree for a task
func GetDepTree(taskID string, allTasks []*model.Task) *DepTreeNode {
	return getDepTreeInner(taskID, allTasks, map[string]bool{})
}

func getDepTreeInner(taskID string, allTasks []*model.Task, visited map[string]bool) *DepTreeNode {
	if visited[taskID] {
		return &DepTreeNode{ID: taskID, Circular: true}
	}
	visited[taskID] = true

	task := findTask(taskID, allTasks)
	if task == nil {
		return &DepTreeNode{ID: taskID, Missing: true}
	}

	node := &DepTreeNode{
		ID:     taskID,
		Title:  task.Title,
		Status: task.Status,
	}
	for _, depID := range task.Depends {
		// Create a copy of visited for each branch
		visitedCopy := make(map[string]bool, len(visited))
		for k, v := range visited {
			visitedCopy[k] = v
		}
		node.Children = append(node.Children, getDepTreeInner(depID, allTasks, visitedCopy))
	}
	return node
}

// GetReverseDeps finds tasks that depend on the given task ID
func GetReverseDeps(taskID string, allTasks []*model.Task) []*model.Task {
	var result []*model.Task
	for _, t := range allTasks {
		for _, dep := range t.Depends {
			if dep == taskID {
				result = append(result, t)
				break
			}
		}
	}
	return result
}

// HasCircularDep checks if adding newDeps to taskID would create a cycle
func HasCircularDep(taskID string, newDeps []string, allTasks []*model.Task) bool {
	visited := map[string]bool{}

	var walk func(id string) bool
	walk = func(id string) bool {
		if id == taskID {
			return true
		}
		if visited[id] {
			return false
		}
		visited[id] = true

		var deps []string
		if id == taskID {
			deps = newDeps
		} else {
			t := findTask(id, allTasks)
			if t != nil {
				deps = t.Depends
			}
		}
		for _, d := range deps {
			if walk(d) {
				return true
			}
		}
		return false
	}

	for _, d := range newDeps {
		if walk(d) {
			return true
		}
	}
	return false
}

// NextAvailable returns the next available task sorted by priority
func NextAvailable(allTasks []*model.Task) *model.Task {
	var available []*model.Task
	for _, t := range allTasks {
		if t.Status == "pending" && !IsBlocked(t, allTasks) {
			available = append(available, t)
		}
	}
	SortByPriority(available)
	if len(available) == 0 {
		return nil
	}
	return available[0]
}

// SortByPriority sorts tasks by priority (critical first)
func SortByPriority(tasks []*model.Task) {
	for i := 0; i < len(tasks); i++ {
		for j := i + 1; j < len(tasks); j++ {
			pi := model.PriorityOrder[tasks[i].Priority]
			pj := model.PriorityOrder[tasks[j].Priority]
			if pi == 0 && tasks[i].Priority == "" {
				pi = 2 // default medium
			}
			if pj == 0 && tasks[j].Priority == "" {
				pj = 2
			}
			if pj < pi {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
}

// PriorityVal returns the numeric priority value, defaulting to medium
func PriorityVal(p string) int {
	v, ok := model.PriorityOrder[p]
	if !ok {
		return 2 // medium
	}
	return v
}

func findTask(id string, tasks []*model.Task) *model.Task {
	for _, t := range tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// ValidateBody checks that the body contains required sections
func ValidateBody(body string) []string {
	var errors []string
	if !containsSection(body, "## Description") {
		errors = append(errors, `Body must contain "## Description" section`)
	}
	if !containsSection(body, "## Acceptance Criteria") {
		errors = append(errors, `Body must contain "## Acceptance Criteria" section`)
	}
	return errors
}

func containsSection(body, section string) bool {
	return len(body) > 0 && contains(body, section)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
