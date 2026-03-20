package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/user/tsk/internal/model"
)

// TaskStore handles task file CRUD operations
type TaskStore struct {
	ItemsDir string
}

// NewTaskStore creates a new TaskStore
func NewTaskStore(itemsDir string) *TaskStore {
	return &TaskStore{ItemsDir: itemsDir}
}

// Read reads a single task by ID
func (s *TaskStore) Read(id string) (*model.Task, error) {
	file := filepath.Join(s.ItemsDir, id+".md")
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	meta, body := ParseFrontmatter(string(data))
	task := &model.Task{
		ID:        GetString(meta, "id"),
		Title:     GetString(meta, "title"),
		Status:    GetString(meta, "status"),
		Phase:     GetString(meta, "phase"),
		Feature:   GetString(meta, "feature"),
		Priority:  GetString(meta, "priority"),
		Depends:   GetStringSlice(meta, "depends"),
		Spec:      GetString(meta, "spec"),
		Files:     GetStringSlice(meta, "files"),
		Created:   GetString(meta, "created"),
		Started:   GetString(meta, "started"),
		Completed: GetString(meta, "completed"),
		Body:      body,
		FilePath:  file,
	}
	if task.ID == "" {
		task.ID = id
	}
	if task.Depends == nil {
		task.Depends = []string{}
	}
	if task.Files == nil {
		task.Files = []string{}
	}

	return task, nil
}

// Write writes a task to its file
func (s *TaskStore) Write(task *model.Task) error {
	content := SerializeFrontmatter(task.MetaMap(), task.Body)
	return os.WriteFile(task.FilePath, []byte(content), 0644)
}

// All returns all tasks sorted by numeric ID
func (s *TaskStore) All() ([]*model.Task, error) {
	if _, err := os.Stat(s.ItemsDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(s.ItemsDir)
	if err != nil {
		return nil, err
	}

	var tasks []*model.Task
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		task, err := s.Read(id)
		if err != nil || task == nil {
			continue
		}
		tasks = append(tasks, task)
	}

	sort.Slice(tasks, func(i, j int) bool {
		return extractNum(tasks[i].ID) < extractNum(tasks[j].ID)
	})

	return tasks, nil
}

// NextID returns the next available task ID
func (s *TaskStore) NextID() (string, error) {
	tasks, err := s.All()
	if err != nil {
		return "", err
	}
	if len(tasks) == 0 {
		return "TASK-001", nil
	}

	maxNum := 0
	for _, t := range tasks {
		n := extractNum(t.ID)
		if n > maxNum {
			maxNum = n
		}
	}
	return fmt.Sprintf("TASK-%03d", maxNum+1), nil
}

// Delete removes a task file
func (s *TaskStore) Delete(id string) error {
	file := filepath.Join(s.ItemsDir, id+".md")
	return os.Remove(file)
}

var numRegex = regexp.MustCompile(`\d+`)

func extractNum(id string) int {
	matches := numRegex.FindAllString(id, -1)
	if len(matches) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(matches[len(matches)-1])
	return n
}
