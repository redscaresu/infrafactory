package runstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

const DefaultRoot = ".infrafactory/runs"

type RunMetadata struct {
	Scenario  string    `json:"scenario"`
	RunID     string    `json:"run_id"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
}

type FilesystemStore struct {
	Root string
}

func NewFilesystemStore(root string) *FilesystemStore {
	if root == "" {
		root = DefaultRoot
	}
	return &FilesystemStore{Root: root}
}

func (s *FilesystemStore) WriteRunMetadata(meta RunMetadata) error {
	if meta.Scenario == "" || meta.RunID == "" {
		return fmt.Errorf("scenario and run_id are required")
	}

	runDir := s.runDir(meta.Scenario, meta.RunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}

	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(runDir, "run.json"), payload, 0o644); err != nil {
		return fmt.Errorf("write run metadata: %w", err)
	}

	return nil
}

func (s *FilesystemStore) ReadRunMetadata(scenario, runID string) (RunMetadata, error) {
	payload, err := os.ReadFile(filepath.Join(s.runDir(scenario, runID), "run.json"))
	if err != nil {
		return RunMetadata{}, fmt.Errorf("read run metadata: %w", err)
	}

	var meta RunMetadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		return RunMetadata{}, fmt.Errorf("decode run metadata: %w", err)
	}

	return meta, nil
}

func (s *FilesystemStore) ListRuns(scenario string) ([]RunMetadata, error) {
	scenarioDir := filepath.Join(s.Root, scenario)
	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read scenario runs: %w", err)
	}

	metas := make([]RunMetadata, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.ReadRunMetadata(scenario, entry.Name())
		if err != nil {
			return nil, err
		}
		metas = append(metas, meta)
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].RunID < metas[j].RunID
	})

	return metas, nil
}

func (s *FilesystemStore) WriteIterationArtifact(scenario, runID string, iteration int, name string, payload []byte) error {
	if scenario == "" || runID == "" || iteration < 1 || name == "" {
		return fmt.Errorf("scenario, run_id, iteration>=1 and name are required")
	}

	artifactDir := filepath.Join(s.runDir(scenario, runID), "iterations", strconv.Itoa(iteration))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(artifactDir, name), payload, 0o644); err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}

	return nil
}

func (s *FilesystemStore) runDir(scenario, runID string) string {
	return filepath.Join(s.Root, scenario, runID)
}
