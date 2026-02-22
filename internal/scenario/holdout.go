package scenario

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type HoldoutScenario struct {
	Path         string
	ScenarioName string
	References   string
}

func DiscoverCriteriaOnlyHoldouts(holdoutDir string, trainingScenarioPath string) ([]HoldoutScenario, error) {
	holdouts := make([]HoldoutScenario, 0)

	err := filepath.WalkDir(holdoutDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		payload, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read holdout %q: %w", path, err)
		}

		var doc struct {
			Scenario   string         `yaml:"scenario"`
			Type       string         `yaml:"type"`
			References string         `yaml:"references"`
			Resources  map[string]any `yaml:"resources"`
		}
		if err := yaml.Unmarshal(payload, &doc); err != nil {
			return fmt.Errorf("parse holdout %q: %w", path, err)
		}

		if doc.Type != "holdout" {
			return nil
		}
		if doc.References != trainingScenarioPath {
			return nil
		}
		if len(doc.Resources) > 0 {
			return nil
		}

		holdouts = append(holdouts, HoldoutScenario{
			Path:         path,
			ScenarioName: doc.Scenario,
			References:   doc.References,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(holdouts, func(i, j int) bool {
		return holdouts[i].Path < holdouts[j].Path
	})

	return holdouts, nil
}
