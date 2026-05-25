package generator

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestPromptTemplateFieldsExistOnPromptContext walks every
// prompts/<cloud>/*.md template in the repo, extracts every Go
// template action that references a field on the context root
// (i.e. `{{.X}}`, `{{if .X}}`, `{{range .X}}`, etc.), and verifies
// that X is an actual exported field on PromptContext.
//
// This test would have caught (and now prevents the regression of):
//   - {{.ArchitectureJSON}} in prompts/aws/phase2 — field is named
//     ArchitecturePlan; the AWS prompt was stale post-rename, but
//     no test exercised it because the e2e suite bypasses the LLM
//     via GeneratorFiles. The text/template missingkey=error setting
//     errors on every Claude generate-phase call.
//   - {{.PitfallsTable}} in prompts/aws/phase2 + phase3 — field is
//     named Pitfalls.
//   - {{.GeneratedHCL}} in prompts/aws/phase3 — field is named
//     GeneratedFiles.
//
// The test is intentionally template-syntax-aware (uses a regex tuned
// to Go's text/template grammar) rather than substring-grep so it
// doesn't false-positive on `.X` inside string literals or comments.
func TestPromptTemplateFieldsExistOnPromptContext(t *testing.T) {
	repoRoot := findRepoRoot(t)
	promptsDir := filepath.Join(repoRoot, "prompts")
	if _, err := os.Stat(promptsDir); err != nil {
		t.Skipf("prompts dir not found at %s — running outside a checked-out repo?", promptsDir)
	}

	validFields := exportedFieldNames(reflect.TypeOf(PromptContext{}))
	fieldRefRegex := regexp.MustCompile(`\{\{[^{}]*?\.([A-Z][A-Za-z0-9_]*)`)

	walkErr := filepath.Walk(promptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		seen := map[string]struct{}{}
		for _, match := range fieldRefRegex.FindAllStringSubmatch(string(body), -1) {
			fieldName := match[1]
			if _, dup := seen[fieldName]; dup {
				continue
			}
			seen[fieldName] = struct{}{}
			if _, ok := validFields[fieldName]; !ok {
				t.Errorf("%s references {{...%s}} but %s is not an exported field on generator.PromptContext (valid fields: %s)",
					rel, fieldName, fieldName, sortedFieldList(validFields))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk prompts dir: %v", walkErr)
	}
}

// TestPromptTemplatesRenderAgainstZeroValueContext renders every
// prompt template with the zero-value PromptContext to catch missing-
// key errors that the field-name test above might miss (e.g. a
// transitive `{{.NestedThing.Field}}` reference where the outer field
// exists but the inner doesn't). Uses the same missingkey=error
// setting RenderPromptTemplate does, so the test mirrors what the
// runtime would see.
//
// Templates that conditionally branch on a field via `{{if .X}}` are
// fine — the if evaluates to false on the zero value and the inner
// block is skipped. We only fail on templates that unconditionally
// reference a non-existent field.
func TestPromptTemplatesRenderAgainstZeroValueContext(t *testing.T) {
	repoRoot := findRepoRoot(t)
	promptsDir := filepath.Join(repoRoot, "prompts")
	if _, err := os.Stat(promptsDir); err != nil {
		t.Skipf("prompts dir not found at %s — running outside a checked-out repo?", promptsDir)
	}

	walkErr := filepath.Walk(promptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			return nil
		}
		if _, err := RenderPromptTemplate(rel, string(body), PromptContext{}); err != nil {
			t.Errorf("%s failed to render with zero-value PromptContext: %v", rel, err)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk prompts dir: %v", walkErr)
	}
}

// TestPromptTemplatesPerCloudPhaseSet asserts that every cloud
// shipped under prompts/ has the same three phase templates — so a
// future cloud addition doesn't drift on which phases exist or what
// they're named. Mirrors the Phase{1,2,3} contract documented in
// generator/SupportedPhases.
func TestPromptTemplatesPerCloudPhaseSet(t *testing.T) {
	repoRoot := findRepoRoot(t)
	promptsDir := filepath.Join(repoRoot, "prompts")
	if _, err := os.Stat(promptsDir); err != nil {
		t.Skipf("prompts dir not found at %s", promptsDir)
	}

	cloudDirs, err := os.ReadDir(promptsDir)
	if err != nil {
		t.Fatalf("read prompts dir: %v", err)
	}

	expectedPhases := map[string]struct{}{
		"phase1_plan_architecture.md": {},
		"phase2_generate_hcl.md":      {},
		"phase3_self_review.md":       {},
	}

	for _, cloud := range cloudDirs {
		if !cloud.IsDir() {
			continue
		}
		cloudPath := filepath.Join(promptsDir, cloud.Name())
		entries, err := os.ReadDir(cloudPath)
		if err != nil {
			t.Errorf("read %s: %v", cloudPath, err)
			continue
		}
		got := map[string]struct{}{}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				got[e.Name()] = struct{}{}
			}
		}
		for phase := range expectedPhases {
			if _, ok := got[phase]; !ok {
				t.Errorf("prompts/%s/ is missing required template %q (every cloud must ship the same three-phase set)", cloud.Name(), phase)
			}
		}
	}
}

// exportedFieldNames returns the set of exported field names on a
// struct type. Used by the field-validity test above.
func exportedFieldNames(t reflect.Type) map[string]struct{} {
	out := map[string]struct{}{}
	if t.Kind() != reflect.Struct {
		return out
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.IsExported() {
			out[f.Name] = struct{}{}
		}
	}
	return out
}

func sortedFieldList(fields map[string]struct{}) string {
	out := make([]string, 0, len(fields))
	for k := range fields {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// findRepoRoot walks up from the current test file until it finds a
// directory containing go.mod. Returns the absolute path. Tests in
// this package run with cwd=internal/generator, so the repo root is
// two levels up.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %s", cwd)
		}
		dir = parent
	}
}
