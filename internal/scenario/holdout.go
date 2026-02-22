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

		var docNode yaml.Node
		if err := yaml.Unmarshal(payload, &docNode); err != nil {
			return fmt.Errorf("parse holdout %q: %w", path, err)
		}
		if len(docNode.Content) == 0 {
			return nil
		}
		rootNode := docNode.Content[0]

		var doc struct {
			Scenario   string `yaml:"scenario"`
			Type       string `yaml:"type"`
			References string `yaml:"references"`
		}
		if err := rootNode.Decode(&doc); err != nil {
			return fmt.Errorf("decode holdout %q: %w", path, err)
		}

		if doc.Type != "holdout" {
			return nil
		}
		if filepath.Clean(doc.References) != filepath.Clean(trainingScenarioPath) {
			return nil
		}
		if hasMappingKey(rootNode, "resources") {
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

func hasMappingKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}

	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}

	return false
}
