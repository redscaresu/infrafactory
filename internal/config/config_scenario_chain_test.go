package config_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

func TestConfigScenarioLoadChain(t *testing.T) {
	t.Parallel()

	schemaPath := filepath.Join("..", "..", "scenario.schema.json")

	cases := []struct {
		name                string
		configPath          string
		scenarioPath        string
		expectedConfigError error
		expectedScenarioErr error
	}{
		{
			name:         "valid config and scenario",
			configPath:   filepath.Join("testdata", "valid.yaml"),
			scenarioPath: filepath.Join("..", "scenario", "testdata", "valid.yaml"),
		},
		{
			name:                "invalid config short-circuits chain",
			configPath:          filepath.Join("testdata", "missing-required.yaml"),
			scenarioPath:        filepath.Join("..", "scenario", "testdata", "valid.yaml"),
			expectedConfigError: config.ErrInvalidConfig,
		},
		{
			name:                "invalid scenario schema after valid config",
			configPath:          filepath.Join("testdata", "valid.yaml"),
			scenarioPath:        filepath.Join("..", "scenario", "testdata", "invalid-schema.yaml"),
			expectedScenarioErr: scenario.ErrInvalidScenario,
		},
		{
			name:                "malformed scenario after valid config",
			configPath:          filepath.Join("testdata", "valid.yaml"),
			scenarioPath:        filepath.Join("..", "scenario", "testdata", "malformed.yaml"),
			expectedScenarioErr: scenario.ErrMalformedScenario,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfgErr, scenarioErr := loadChain(tc.configPath, tc.scenarioPath, schemaPath)
			if !errors.Is(cfgErr, tc.expectedConfigError) {
				t.Fatalf("expected config error %v, got %v", tc.expectedConfigError, cfgErr)
			}
			if !errors.Is(scenarioErr, tc.expectedScenarioErr) {
				t.Fatalf("expected scenario error %v, got %v", tc.expectedScenarioErr, scenarioErr)
			}
		})
	}
}

func loadChain(configPath, scenarioPath, schemaPath string) (error, error) {
	if _, err := config.Load(configPath); err != nil {
		return err, nil
	}

	if _, err := scenario.LoadWithSchema(scenarioPath, schemaPath); err != nil {
		return nil, err
	}

	return nil, nil
}
