package harness

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var ErrSandboxDestroyFailed = errors.New("sandbox destroy failed")

type SandboxDestroyHarness struct {
	runner CommandRunner
}

func NewSandboxDestroyHarness(runner CommandRunner) *SandboxDestroyHarness {
	return &SandboxDestroyHarness{runner: runner}
}

type SandboxDestroyResult struct {
	Destroy StageResult
	// WithoutConfig is set when the resources were destroyed without the
	// workdir's configuration, and holds why: the ordinary destroy's
	// failure. A teardown that quietly took another route would read
	// exactly like one that never needed to.
	WithoutConfig string
}

type SandboxDestroyError struct {
	Stage   string
	Destroy StageResult
	Err     error
}

func (e *SandboxDestroyError) Error() string {
	if e == nil {
		return ErrSandboxDestroyFailed.Error()
	}
	return fmt.Sprintf("%s: %s: %v", ErrSandboxDestroyFailed, e.Stage, e.Err)
}

func (e *SandboxDestroyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *SandboxDestroyError) Is(target error) bool {
	return target == ErrSandboxDestroyFailed
}

func (h *SandboxDestroyHarness) Run(ctx context.Context, workDir string, env map[string]string) (*SandboxDestroyResult, error) {
	cmd := Command{
		Name:     "tofu",
		Args:     []string{"destroy", "-auto-approve", "-state=" + LiveStateFilename},
		Dir:      workDir,
		Env:      env,
		StripEnv: SandboxStripEnv,
	}
	destroyResult, err := h.runner.Run(ctx, cmd)
	stage := StageResult{
		Stage:  "destroy",
		Cmd:    []string{"tofu", "destroy", "-auto-approve", "-state=" + LiveStateFilename},
		Stdout: string(destroyResult.Stdout),
		Stderr: string(destroyResult.Stderr),
	}
	if err != nil {
		return nil, &SandboxDestroyError{
			Stage:   "destroy",
			Destroy: stage,
			Err:     err,
		}
	}

	return &SandboxDestroyResult{Destroy: stage}, nil
}

// WithoutConfigStage names the fallback destroy in stages and errors.
const WithoutConfigStage = "destroy_without_config"

// RunWithoutConfig destroys everything the live state records without
// evaluating the workdir's configuration.
//
// `tofu destroy` evaluates the whole configuration before it deletes
// anything. A configuration that cannot evaluate -- the one whose apply
// created half a stack and then failed on an expression -- fails the
// destroy the same way, and the stack outlives the run. Against a
// configuration that declares no resources, every resource in the state
// is an orphan, and tofu destroys orphans from the state alone, in
// dependency order, through the same provider.
//
// That configuration lives in a temporary directory: the provider
// requirements the state names and the workdir's lock file. TF_DATA_DIR
// points at the workdir's .terraform, so the providers `init` installed
// are reused rather than downloaded. The workdir itself is not touched
// except for its state file.
//
// It refuses when the state records anything it cannot place in
// projectID. Nothing the configuration says -- prevent_destroy included
// -- applies here, so this deletes only the run's own project's contents.
func (h *SandboxDestroyHarness) RunWithoutConfig(ctx context.Context, workDir, projectID string, env map[string]string) (*SandboxDestroyResult, error) {
	fail := func(err error) (*SandboxDestroyResult, error) {
		return nil, &SandboxDestroyError{Stage: WithoutConfigStage, Err: err}
	}
	if strings.TrimSpace(projectID) == "" {
		return fail(fmt.Errorf("%w: no run project to scope it to", ErrProtectedProject))
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return fail(err)
	}
	statePath := filepath.Join(absWorkDir, LiveStateFilename)
	state, err := loadLiveTerraformState(statePath)
	if err != nil {
		return fail(err)
	}
	if unplaced := unplacedResources(state, projectID); len(unplaced) > 0 {
		return fail(fmt.Errorf("%w: the state records resources that cannot be placed in run project %s: %s",
			ErrProtectedProject, projectID, strings.Join(unplaced, "; ")))
	}

	configDir, err := os.MkdirTemp("", "infrafactory-teardown-")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(configDir)
	if err := os.WriteFile(filepath.Join(configDir, "main.tf"), []byte(teardownConfig(state)), 0o600); err != nil {
		return fail(err)
	}
	lock, err := os.ReadFile(filepath.Join(absWorkDir, ".terraform.lock.hcl"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	if err == nil {
		if err := os.WriteFile(filepath.Join(configDir, ".terraform.lock.hcl"), lock, 0o600); err != nil {
			return fail(err)
		}
	}

	cmdEnv := maps.Clone(env)
	if cmdEnv == nil {
		cmdEnv = map[string]string{}
	}
	cmdEnv["TF_DATA_DIR"] = filepath.Join(absWorkDir, ".terraform")
	args := []string{"destroy", "-auto-approve", "-state=" + statePath}
	out, err := h.runner.Run(ctx, Command{Name: "tofu", Args: args, Dir: configDir, Env: cmdEnv, StripEnv: SandboxStripEnv})
	stage := StageResult{
		Stage:  WithoutConfigStage,
		Cmd:    append([]string{"tofu"}, args...),
		Stdout: string(out.Stdout),
		Stderr: string(out.Stderr),
	}
	if err != nil {
		return nil, &SandboxDestroyError{Stage: WithoutConfigStage, Destroy: stage, Err: err}
	}
	return &SandboxDestroyResult{Destroy: stage}, nil
}

// ChildScopedTypes carry no project_id because they live inside a parent
// resource; the value names the attributes that reference the parent.
// The Layer 3 shape gate requires every one of those to reference a
// resource in the same stack, which is what places a child in the run's
// project.
var ChildScopedTypes = map[string][]string{
	"scaleway_lb_backend":           {"lb_id"},
	"scaleway_lb_frontend":          {"lb_id", "backend_id"},
	"scaleway_lb_route":             {"frontend_id", "backend_id"},
	"scaleway_lb_certificate":       {"lb_id"},
	"scaleway_instance_private_nic": {"server_id", "private_network_id"},
}

// unplacedResources names every managed resource the state cannot place
// in projectID. An allowlist, so no type slips through by omission: a
// resource is placed by recording projectID as its project_id, or, for a
// ChildScopedTypes child, by every parent it names being a resource in
// the same state -- any parent that is not placed refuses the whole
// destroy on its own. Everything else is refused: another project's
// resource, a project, an IAM object. strayResourceFailures is looser
// because for the sweep a missing project_id is not evidence of a stray.
//
// This judges the state, which is a local file. It catches stale and
// foreign content; it cannot catch a forged project_id, and neither can
// the ordinary destroy, which trusts the same file. The API-stamp check
// the caller runs first is what protects projects the run did not make.
func unplacedResources(state terraformState, projectID string) []string {
	// Parents count only if this state manages them: a data source's id
	// names something the run looked up, not something it owns.
	ids := map[string]bool{}
	for _, resource := range state.Resources {
		if !managedCloudResource(resource) {
			continue
		}
		for _, instance := range resource.Instances {
			if id, _ := instance.Attributes["id"].(string); id != "" {
				ids[path.Base(id)] = true
			}
		}
	}

	var unplaced []string
	for _, resource := range state.Resources {
		if !managedCloudResource(resource) {
			continue
		}
		parents, child := ChildScopedTypes[resource.Type]
		for _, instance := range resource.Instances {
			id, _ := instance.Attributes["id"].(string)
			if !child {
				if got, _ := instance.Attributes["project_id"].(string); got != projectID {
					unplaced = append(unplaced, fmt.Sprintf("%s (%s) in project %q", resource.Type, id, got))
				}
				continue
			}
			for _, attr := range parents {
				// Compared by the trailing UUID: a child can name its
				// parent with a different locality prefix than the
				// parent's own id carries.
				if parent, _ := instance.Attributes[attr].(string); parent == "" || !ids[path.Base(parent)] {
					unplaced = append(unplaced, fmt.Sprintf("%s (%s) whose %s %q is not in the state", resource.Type, id, attr, parent))
				}
			}
		}
	}
	return unplaced
}

// managedCloudResource is a resource the destroy would delete from a
// cloud: not a data source, and not a builtin like terraform_data.
func managedCloudResource(resource terraformResource) bool {
	return resource.Mode != "data" && !strings.HasPrefix(resource.Provider, `provider["terraform.io/builtin/`)
}

// rootProviderAddr matches a state resource's provider when it is
// configured in the root module: provider["<source>"] or
// provider["<source>"].<alias>.
var rootProviderAddr = regexp.MustCompile(`^provider\["([^"]+)"\](?:\.([A-Za-z0-9_-]+))?$`)

// teardownConfig is a configuration with no resources: a requirement and
// a provider block, per alias, for every provider the state names. The
// provider blocks are empty; the sandbox env supplies credentials,
// region, zone and project. Builtin providers (terraform_data) need
// neither. A provider configured inside a module is not reproduced, so
// tofu refuses loudly rather than this guessing.
func teardownConfig(state terraformState) string {
	sources := map[string]string{}
	blocks := map[string]bool{}
	for _, resource := range state.Resources {
		m := rootProviderAddr.FindStringSubmatch(resource.Provider)
		if m == nil || strings.HasPrefix(m[1], "terraform.io/builtin/") {
			continue
		}
		name := path.Base(m[1])
		sources[name] = m[1]
		block := fmt.Sprintf("provider %q {}\n", name)
		if m[2] != "" {
			block = fmt.Sprintf("provider %q {\n  alias = %q\n}\n", name, m[2])
		}
		blocks[block] = true
	}

	var b strings.Builder
	b.WriteString("terraform {\n  required_providers {\n")
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		fmt.Fprintf(&b, "    %s = { source = %q }\n", name, sources[name])
	}
	b.WriteString("  }\n}\n")
	for _, block := range slices.Sorted(maps.Keys(blocks)) {
		b.WriteString("\n" + block)
	}
	return b.String()
}
