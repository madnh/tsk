package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/madnh/tsk/internal/model"
)

// LoopStore handles loop state file operations
type LoopStore struct {
	LoopDir string
}

// NewLoopStore creates a new LoopStore
func NewLoopStore(loopDir string) *LoopStore {
	return &LoopStore{LoopDir: loopDir}
}

// StateFile returns the path to state.json
func (s *LoopStore) StateFile() string {
	return filepath.Join(s.LoopDir, "state.json")
}

// LogFile returns the path to history.log
func (s *LoopStore) LogFile() string {
	return filepath.Join(s.LoopDir, "history.log")
}

// ReadState reads the loop state
func (s *LoopStore) ReadState() (*model.LoopState, error) {
	data, err := os.ReadFile(s.StateFile())
	if err != nil {
		return nil, err
	}
	var state model.LoopState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// WriteState writes the loop state
func (s *LoopStore) WriteState(state *model.LoopState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.StateFile(), data, 0644)
}

// StateExists checks if state.json exists
func (s *LoopStore) StateExists() bool {
	_, err := os.Stat(s.StateFile())
	return err == nil
}

// Log appends a log entry with timestamp
func (s *LoopStore) Log(entry string) {
	line := fmt.Sprintf("[%s] %s\n", localTimestamp(), entry)
	f, err := os.OpenFile(s.LogFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}

// ReadFile reads a file from the loop directory
func (s *LoopStore) ReadFile(name string) string {
	data, err := os.ReadFile(filepath.Join(s.LoopDir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WriteFile writes a file to the loop directory
func (s *LoopStore) WriteFile(name, content string) error {
	return os.WriteFile(filepath.Join(s.LoopDir, name), []byte(content), 0644)
}

// DeleteFile removes a file from the loop directory
func (s *LoopStore) DeleteFile(name string) {
	os.Remove(filepath.Join(s.LoopDir, name))
}

// FileExists checks if a file exists in the loop directory
func (s *LoopStore) FileExists(name string) bool {
	_, err := os.Stat(filepath.Join(s.LoopDir, name))
	return err == nil
}

// ReadLogEntries reads all log entries
func (s *LoopStore) ReadLogEntries() ([]string, error) {
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

// EnsureDir creates the loop directory if it doesn't exist
func (s *LoopStore) EnsureDir() error {
	return os.MkdirAll(s.LoopDir, 0755)
}

// Reset clears loop state files but preserves history.log and prompts/
func (s *LoopStore) Reset() error {
	entries, err := os.ReadDir(s.LoopDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	preserve := map[string]bool{
		"history.log": true,
		"prompts":     true,
	}

	for _, e := range entries {
		if preserve[e.Name()] {
			continue
		}
		if e.IsDir() {
			continue
		}
		os.Remove(filepath.Join(s.LoopDir, e.Name()))
	}
	return nil
}

func localTimestamp() string {
	now := time.Now()
	return now.Format("2006-01-02 15:04:05")
}
