package config

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestLoadValidConfigAppliesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Version != "1.0" {
		t.Fatalf("expected version 1.0, got %q", cfg.Version)
	}
	if cfg.Agent.Type != "claude-code" {
		t.Fatalf("expected agent.type claude-code, got %q", cfg.Agent.Type)
	}
	if cfg.Agent.MaxIterations != 5 {
		t.Fatalf("expected default max_iterations 5, got %d", cfg.Agent.MaxIterations)
	}
	if cfg.Mockway.AutoReset != true {
		t.Fatalf("expected default mockway.auto_reset true, got %v", cfg.Mockway.AutoReset)
	}
	if cfg.Paths.Output != "./output" {
		t.Fatalf("expected default paths.output ./output, got %q", cfg.Paths.Output)
	}
}

func TestLoadMissingRequiredFieldsReturnsTypedValidationError(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join("testdata", "missing-required.yaml"))
	if err == nil {
		t.Fatal("expected validation error")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
	}

	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected errors.Is(..., ErrInvalidConfig) to be true, got %v", err)
	}

	if len(validationErr.Fields) != 3 {
		t.Fatalf("expected 3 field errors, got %d", len(validationErr.Fields))
	}

	expected := map[string]bool{
		"agent.type":  false,
		"mockway.url": false,
		"version":     false,
	}

	for _, field := range validationErr.Fields {
		if _, ok := expected[field.Field]; ok {
			expected[field.Field] = true
		}
	}

	for field, seen := range expected {
		if !seen {
			t.Fatalf("expected validation error for field %q", field)
		}
	}
}
