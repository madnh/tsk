package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/madnh/tsk/internal/model"
)

// SupervisorStore handles supervisor state file operations
type SupervisorStore struct {
	LoopDir string
}

// NewSupervisorStore creates a new SupervisorStore
func NewSupervisorStore(loopDir string) *SupervisorStore {
	return &SupervisorStore{LoopDir: loopDir}
}

// StateFile returns the path to supervisor.json
func (s *SupervisorStore) StateFile() string {
	return filepath.Join(s.LoopDir, "supervisor.json")
}

// LogFile returns the path to supervisor.log
func (s *SupervisorStore) LogFile() string {
	return filepath.Join(s.LoopDir, "supervisor.log")
}

// ReadState reads the supervisor state
func (s *SupervisorStore) ReadState() (*model.SupervisorState, error) {
	data, err := os.ReadFile(s.StateFile())
	if err != nil {
		return nil, err
	}
	var state model.SupervisorState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// WriteState writes the supervisor state
func (s *SupervisorStore) WriteState(state *model.SupervisorState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.StateFile(), data, 0644)
}

// StateExists checks if supervisor.json exists
func (s *SupervisorStore) StateExists() bool {
	_, err := os.Stat(s.StateFile())
	return err == nil
}

// AddWorker adds a new worker entry to the supervisor state
func (s *SupervisorStore) AddWorker(taskID string, pid int) error {
	state, err := s.ReadState()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if state == nil {
		return fmt.Errorf("supervisor state not initialized")
	}

	entry := model.WorkerEntry{
		TaskID:    taskID,
		PID:       pid,
		Status:    "running",
		SpawnedAt: time.Now().Format(time.RFC3339),
	}

	state.Workers = append(state.Workers, entry)
	return s.WriteState(state)
}

// UpdateWorker updates a worker entry's status in the supervisor state
func (s *SupervisorStore) UpdateWorker(taskID, status string) error {
	state, err := s.ReadState()
	if err != nil {
		return err
	}

	for i, w := range state.Workers {
		if w.TaskID == taskID {
			state.Workers[i].Status = status
			return s.WriteState(state)
		}
	}

	return fmt.Errorf("worker entry for task %s not found", taskID)
}

// RemoveWorker removes a worker entry from the supervisor state
func (s *SupervisorStore) RemoveWorker(taskID string) error {
	state, err := s.ReadState()
	if err != nil {
		return err
	}

	for i, w := range state.Workers {
		if w.TaskID == taskID {
			state.Workers = append(state.Workers[:i], state.Workers[i+1:]...)
			return s.WriteState(state)
		}
	}

	return nil
}

// Log appends a log entry with timestamp
func (s *SupervisorStore) Log(entry string) {
	line := fmt.Sprintf("[%s] %s\n", localTimestamp(), entry)
	f, err := os.OpenFile(s.LogFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}

// EnsureDir creates the loop directory if it doesn't exist
func (s *SupervisorStore) EnsureDir() error {
	return os.MkdirAll(s.LoopDir, 0755)
}
