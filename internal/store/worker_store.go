package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/madnh/tsk/internal/model"
)

// WorkerStore handles worker state file operations in tasks/workers/TASK-XXX/
type WorkerStore struct {
	WorkerDir string
}

// NewWorkerStore creates a new WorkerStore for a specific task
func NewWorkerStore(tasksDir, taskID string) *WorkerStore {
	return &WorkerStore{
		WorkerDir: filepath.Join(tasksDir, "workers", taskID),
	}
}

// StateFile returns the path to state.json
func (s *WorkerStore) StateFile() string {
	return filepath.Join(s.WorkerDir, "state.json")
}

// LogFile returns the path to history.log
func (s *WorkerStore) LogFile() string {
	return filepath.Join(s.WorkerDir, "history.log")
}

// ReadState reads the worker state
func (s *WorkerStore) ReadState() (*model.WorkerState, error) {
	data, err := os.ReadFile(s.StateFile())
	if err != nil {
		return nil, err
	}
	var state model.WorkerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// WriteState writes the worker state
func (s *WorkerStore) WriteState(state *model.WorkerState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.StateFile(), data, 0644)
}

// StateExists checks if state.json exists
func (s *WorkerStore) StateExists() bool {
	_, err := os.Stat(s.StateFile())
	return err == nil
}

// Log appends a log entry with timestamp
func (s *WorkerStore) Log(entry string) {
	line := fmt.Sprintf("[%s] %s\n", localTimestamp(), entry)
	f, err := os.OpenFile(s.LogFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}

// ReadFile reads a file from the worker directory
func (s *WorkerStore) ReadFile(name string) string {
	data, err := os.ReadFile(filepath.Join(s.WorkerDir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WriteFile writes a file to the worker directory
func (s *WorkerStore) WriteFile(name, content string) error {
	return os.WriteFile(filepath.Join(s.WorkerDir, name), []byte(content), 0644)
}

// DeleteFile removes a file from the worker directory
func (s *WorkerStore) DeleteFile(name string) {
	os.Remove(filepath.Join(s.WorkerDir, name))
}

// FileExists checks if a file exists in the worker directory
func (s *WorkerStore) FileExists(name string) bool {
	_, err := os.Stat(filepath.Join(s.WorkerDir, name))
	return err == nil
}

// ReadLogEntries reads all log entries
func (s *WorkerStore) ReadLogEntries() ([]string, error) {
	data, err := os.ReadFile(s.LogFile())
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}

// EnsureDir creates the worker directory if it doesn't exist
func (s *WorkerStore) EnsureDir() error {
	return os.MkdirAll(s.WorkerDir, 0755)
}
