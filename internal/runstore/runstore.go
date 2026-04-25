package runstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultRoot = ".infrafactory/runs"

const (
	RunMetadataSchemaVersion  = "infrafactory.run.metadata.v1"
	RunMetadataSchemaLegacy   = "infrafactory.run.metadata.legacy"
	RunIterationSchemaVersion = "infrafactory.run.iteration.v1"
)

type RunMetadata struct {
	Schema              string    `json:"schema,omitempty"`
	Scenario            string    `json:"scenario"`
	RunID               string    `json:"run_id"`
	Status              string    `json:"status"`
	TerminalReason      string    `json:"terminal_reason,omitempty"`
	Incremental         bool      `json:"incremental,omitempty"`
	Layer3Enabled       bool      `json:"layer3_enabled,omitempty"`
	PreviousRunID       string    `json:"previous_run_id,omitempty"`
	RepairIterationsMax int       `json:"repair_iterations_max,omitempty"`
	StartedAt           time.Time `json:"started_at"`
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
	if meta.Schema == "" {
		meta.Schema = RunMetadataSchemaVersion
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
	if meta.Schema == "" {
		meta.Schema = RunMetadataSchemaLegacy
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
		runMetadataPath := filepath.Join(scenarioDir, entry.Name(), "run.json")
		if _, err := os.Stat(runMetadataPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat run metadata: %w", err)
		}
		meta, err := s.ReadRunMetadata(scenario, entry.Name())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
				// Ignore incomplete run directories created before metadata is persisted.
				continue
			}
			return nil, err
		}
		metas = append(metas, meta)
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].RunID > metas[j].RunID
	})

	return metas, nil
}

func (s *FilesystemStore) LatestSuccessfulRunID(scenario string) (string, error) {
	runs, err := s.ListRuns(scenario)
	if err != nil {
		return "", err
	}
	for _, run := range runs {
		if run.Status == "success" {
			return run.RunID, nil
		}
	}
	return "", nil
}

func (s *FilesystemStore) ListScenarios() ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read runstore root: %w", err)
	}

	scenarios := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		scenarios = append(scenarios, entry.Name())
	}
	sort.Strings(scenarios)
	return scenarios, nil
}

func (s *FilesystemStore) WriteIterationArtifact(scenario, runID string, iteration int, name string, payload []byte) error {
	if scenario == "" || runID == "" || iteration < 1 || name == "" {
		return fmt.Errorf("scenario, run_id, iteration>=1 and name are required")
	}

	artifactDir := filepath.Join(s.runDir(scenario, runID), "iterations", strconv.Itoa(iteration))
	target, err := resolveGeneratedPath(artifactDir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}

	if err := os.WriteFile(target, payload, 0o644); err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}

	return nil
}

func (s *FilesystemStore) WriteGeneratedFiles(scenario, runID string, files map[string][]byte) error {
	if scenario == "" || runID == "" {
		return fmt.Errorf("scenario and run_id are required")
	}

	root := s.generatedDir(scenario, runID)
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("reset generated files dir: %w", err)
	}
	return writeGeneratedFiles(root, files)
}

func (s *FilesystemStore) WriteRunArtifact(scenario, runID, name string, payload []byte) error {
	if scenario == "" || runID == "" || name == "" {
		return fmt.Errorf("scenario, run_id and name are required")
	}

	target, err := resolveGeneratedPath(s.runDir(scenario, runID), name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create run artifact dir: %w", err)
	}
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		return fmt.Errorf("write run artifact: %w", err)
	}
	return nil
}

func (s *FilesystemStore) WriteIterationGeneratedFiles(scenario, runID string, iteration int, files map[string][]byte) error {
	if scenario == "" || runID == "" || iteration < 1 {
		return fmt.Errorf("scenario, run_id and iteration>=1 are required")
	}

	root := s.iterationGeneratedDir(scenario, runID, iteration)
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("reset iteration generated files dir: %w", err)
	}
	return writeGeneratedFiles(root, files)
}

func (s *FilesystemStore) ListGeneratedFiles(scenario, runID string) ([]string, error) {
	if scenario == "" || runID == "" {
		return nil, fmt.Errorf("scenario and run_id are required")
	}

	return listGeneratedFiles(s.generatedDir(scenario, runID))
}

func (s *FilesystemStore) ListIterationGeneratedFiles(scenario, runID string, iteration int) ([]string, error) {
	if scenario == "" || runID == "" || iteration < 1 {
		return nil, fmt.Errorf("scenario, run_id and iteration>=1 are required")
	}

	return listGeneratedFiles(s.iterationGeneratedDir(scenario, runID, iteration))
}

func (s *FilesystemStore) ReadGeneratedFile(scenario, runID, relPath string) ([]byte, error) {
	if scenario == "" || runID == "" || relPath == "" {
		return nil, fmt.Errorf("scenario, run_id and path are required")
	}

	target, err := resolveGeneratedPath(s.generatedDir(scenario, runID), relPath)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read generated file: %w", err)
	}
	return payload, nil
}

func (s *FilesystemStore) ReadIterationGeneratedFile(scenario, runID string, iteration int, relPath string) ([]byte, error) {
	if scenario == "" || runID == "" || iteration < 1 || relPath == "" {
		return nil, fmt.Errorf("scenario, run_id, iteration>=1 and path are required")
	}

	target, err := resolveGeneratedPath(s.iterationGeneratedDir(scenario, runID, iteration), relPath)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read generated file: %w", err)
	}
	return payload, nil
}

func (s *FilesystemStore) ListIterations(scenario, runID string) ([]int, error) {
	if scenario == "" || runID == "" {
		return nil, fmt.Errorf("scenario and run_id are required")
	}

	root := filepath.Join(s.runDir(scenario, runID), "iterations")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read iterations dir: %w", err)
	}

	iterations := make([]int, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		n, err := strconv.Atoi(entry.Name())
		if err != nil || n < 1 {
			continue
		}
		iterations = append(iterations, n)
	}
	sort.Ints(iterations)
	return iterations, nil
}

func (s *FilesystemStore) ReadIterationArtifact(scenario, runID string, iteration int) ([]byte, error) {
	if scenario == "" || runID == "" || iteration < 1 {
		return nil, fmt.Errorf("scenario, run_id and iteration>=1 are required")
	}

	path := filepath.Join(s.runDir(scenario, runID), "iterations", strconv.Itoa(iteration), "iteration.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read iteration artifact: %w", err)
	}

	return payload, nil
}

func (s *FilesystemStore) ReadRunLog(scenario, runID string) ([]byte, error) {
	if scenario == "" || runID == "" {
		return nil, fmt.Errorf("scenario and run_id are required")
	}

	path := filepath.Join(s.runDir(scenario, runID), "app.log")
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run log: %w", err)
	}

	return payload, nil
}

func (s *FilesystemStore) ReadRunArtifact(scenario, runID, name string) ([]byte, error) {
	if scenario == "" || runID == "" || name == "" {
		return nil, fmt.Errorf("scenario, run_id and name are required")
	}

	target, err := resolveGeneratedPath(s.runDir(scenario, runID), name)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read run artifact: %w", err)
	}
	return payload, nil
}

func (s *FilesystemStore) runDir(scenario, runID string) string {
	return filepath.Join(s.Root, scenario, runID)
}

func (s *FilesystemStore) generatedDir(scenario, runID string) string {
	return filepath.Join(s.runDir(scenario, runID), "generated")
}

func (s *FilesystemStore) iterationGeneratedDir(scenario, runID string, iteration int) string {
	return filepath.Join(s.runDir(scenario, runID), "iterations", strconv.Itoa(iteration), "generated")
}

func writeGeneratedFiles(root string, files map[string][]byte) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create generated files dir: %w", err)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		targetPath, err := resolveGeneratedPath(root, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("create generated file dir: %w", err)
		}
		if err := os.WriteFile(targetPath, files[name], 0o644); err != nil {
			return fmt.Errorf("write generated file: %w", err)
		}
	}

	return nil
}

func listGeneratedFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "..") {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list generated files: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func resolveGeneratedPath(root, relPath string) (string, error) {
	cleanPath := filepath.Clean(relPath)
	if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("invalid generated file path %q", relPath)
	}

	target := filepath.Join(root, cleanPath)
	absRoot, _ := filepath.Abs(root)
	absTarget, _ := filepath.Abs(target)
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid generated file path %q", relPath)
	}
	return absTarget, nil
}
