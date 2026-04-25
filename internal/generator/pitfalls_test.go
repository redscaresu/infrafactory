package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPitfallsScaleway(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	yaml := `provider: scaleway
pitfalls:
  - resource: scaleway_instance_server
    rule: Always set enable_ipv6 explicitly
    source: docs
  - resource: scaleway_rdb_instance
    rule: node_type must match region availability
    source: experience
`
	if err := os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadPitfalls(dir, "scaleway")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "scaleway_instance_server") {
		t.Errorf("expected scaleway_instance_server in output, got %q", got)
	}
	if !strings.Contains(got, "Always set enable_ipv6 explicitly") {
		t.Errorf("expected first rule in output, got %q", got)
	}
	if !strings.Contains(got, "scaleway_rdb_instance") {
		t.Errorf("expected scaleway_rdb_instance in output, got %q", got)
	}
	if !strings.Contains(got, "node_type must match region availability") {
		t.Errorf("expected second rule in output, got %q", got)
	}
}

func TestLoadPitfallsWithCommon(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	providerYAML := `provider: scaleway
pitfalls:
  - resource: scaleway_vpc
    rule: Always specify region
    source: docs
`
	commonYAML := `provider: common
pitfalls:
  - resource: all
    rule: Use lifecycle ignore_changes for volatile fields
    source: best-practice
`
	if err := os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte(providerYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "common.yaml"), []byte(commonYAML), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadPitfalls(dir, "scaleway")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "scaleway_vpc") {
		t.Errorf("expected provider pitfall in output, got %q", got)
	}
	if !strings.Contains(got, "Always specify region") {
		t.Errorf("expected provider rule in output, got %q", got)
	}
	if !strings.Contains(got, "all") {
		t.Errorf("expected common resource in output, got %q", got)
	}
	if !strings.Contains(got, "Use lifecycle ignore_changes for volatile fields") {
		t.Errorf("expected common rule in output, got %q", got)
	}
}

func TestLoadPitfallsMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	got, err := LoadPitfalls(dir, "nonexistent")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for missing file, got %q", got)
	}
}

func TestLoadPitfallsEmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadPitfalls(dir, "scaleway")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for empty file, got %q", got)
	}
}

func TestLoadPitfallsRendersMarkdownBullets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	yaml := `provider: scaleway
pitfalls:
  - resource: scaleway_instance_server
    rule: Set boot_type to local
    source: docs
  - resource: scaleway_lb
    rule: Assign to private network before creating backends
    source: experience
`
	if err := os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadPitfalls(dir, "scaleway")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "- `") {
			t.Errorf("expected line to start with '- `', got %q", line)
		}
		if !strings.Contains(line, "`: ") {
			t.Errorf("expected line to contain '`: ' separator, got %q", line)
		}
	}

	if !strings.Contains(got, "- `scaleway_instance_server`: Set boot_type to local") {
		t.Errorf("expected exact bullet format, got %q", got)
	}
	if !strings.Contains(got, "- `scaleway_lb`: Assign to private network before creating backends") {
		t.Errorf("expected exact bullet format, got %q", got)
	}
}

func TestLoadPitfallsMultipleSameResource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	yaml := `provider: scaleway
pitfalls:
  - resource: scaleway_instance_server
    rule: Set boot_type to local
    source: docs
  - resource: scaleway_instance_server
    rule: Always specify image
    source: experience
  - resource: scaleway_instance_server
    rule: Use cloud-init for provisioning
    source: best-practice
`
	if err := os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadPitfalls(dir, "scaleway")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "Set boot_type to local") {
		t.Errorf("expected first rule in output, got %q", got)
	}
	if !strings.Contains(got, "Always specify image") {
		t.Errorf("expected second rule in output, got %q", got)
	}
	if !strings.Contains(got, "Use cloud-init for provisioning") {
		t.Errorf("expected third rule in output, got %q", got)
	}

	// All three should be for the same resource.
	count := strings.Count(got, "`scaleway_instance_server`")
	if count != 3 {
		t.Errorf("expected 3 bullets for scaleway_instance_server, got %d in %q", count, got)
	}
}
