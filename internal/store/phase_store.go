package store

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/user/tsk/internal/model"
)

// PhaseStore handles phase file CRUD
type PhaseStore struct {
	PhasesDir string
}

// NewPhaseStore creates a new PhaseStore
func NewPhaseStore(phasesDir string) *PhaseStore {
	return &PhaseStore{PhasesDir: phasesDir}
}

var phaseFileRegex = regexp.MustCompile(`^phase-(\d+)\.md$`)

// All returns all phases sorted by number
func (s *PhaseStore) All() ([]*model.Phase, error) {
	if _, err := os.Stat(s.PhasesDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(s.PhasesDir)
	if err != nil {
		return nil, err
	}

	var phases []*model.Phase
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		matches := phaseFileRegex.FindStringSubmatch(e.Name())
		if matches == nil {
			continue
		}

		num := matches[1]
		fp := filepath.Join(s.PhasesDir, e.Name())
		data, err := os.ReadFile(fp)
		if err != nil {
			continue
		}

		meta, body := ParseFrontmatter(string(data))
		phase := &model.Phase{
			Num:         num,
			Name:        GetString(meta, "name"),
			Description: GetString(meta, "description"),
			Status:      GetString(meta, "status"),
			Body:        body,
			FilePath:    fp,
			RawMeta:     make(map[string]string),
		}
		if phase.Name == "" {
			phase.Name = "Phase " + num
		}
		if phase.Status == "" {
			phase.Status = "defining"
		}
		// Store raw meta for preservation
		for k, v := range meta {
			if s, ok := v.(string); ok {
				phase.RawMeta[k] = s
			}
		}
		phases = append(phases, phase)
	}

	sort.Slice(phases, func(i, j int) bool {
		return extractNum(phases[i].Num) < extractNum(phases[j].Num)
	})

	return phases, nil
}

// Write writes a phase to its file
func (s *PhaseStore) Write(phase *model.Phase) error {
	// Build KVs preserving all raw meta but updating known fields
	rawCopy := make(map[string]string)
	for k, v := range phase.RawMeta {
		rawCopy[k] = v
	}
	rawCopy["name"] = phase.Name
	rawCopy["description"] = phase.Description
	rawCopy["status"] = phase.Status

	// Build ordered KVs: name, description, status first, then rest
	var kvs []model.KV
	ordered := []string{"name", "description", "status"}
	seen := map[string]bool{}
	for _, k := range ordered {
		if v, ok := rawCopy[k]; ok {
			kvs = append(kvs, model.KV{Key: k, Value: v})
			seen[k] = true
		}
	}
	for k, v := range rawCopy {
		if !seen[k] {
			kvs = append(kvs, model.KV{Key: k, Value: v})
		}
	}

	content := SerializeFrontmatter(kvs, phase.Body)
	return os.WriteFile(phase.FilePath, []byte(content), 0644)
}

// Find finds a phase by number
func (s *PhaseStore) Find(num string) (*model.Phase, error) {
	phases, err := s.All()
	if err != nil {
		return nil, err
	}
	for _, p := range phases {
		if p.Num == num {
			return p, nil
		}
	}
	return nil, nil
}

// ReadBody reads the body portion of a phase, trimmed
func ReadBody(phase *model.Phase) string {
	return strings.TrimSpace(phase.Body)
}
