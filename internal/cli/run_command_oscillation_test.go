package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// alternatingStaticHarness fails with one of two errors based on the
// 0-indexed call count modulo 2. Used to simulate the model alternating
// between two incomplete fixes (oscillation).
type alternatingStaticHarness struct {
	calls int
	errs  [2]error
}

func (a *alternatingStaticHarness) Run(ctx context.Context, workDir string, env map[string]string) (*harness.StaticResult, error) {
	idx := a.calls % 2
	a.calls++
	return nil, a.errs[idx]
}

// TestRunCommandLearnsPitfallFromOscillation drives the run loop with a
// stub static harness that alternates between two distinct failures over
// four iterations (A, B, A, B), exhausts the repair budget, and verifies
// the run loop extracts a pitfall from the oscillating signature whose
// detail matches a known ExtractDescriptivePitfall pattern.
func TestRunCommandLearnsPitfallFromOscillation(t *testing.T) {
	h := newCommandTestHarness(t)

	pitfallsDir := filepath.Join(h.WorkspaceDir, "pitfalls")
	if err := os.MkdirAll(pitfallsDir, 0o755); err != nil {
		t.Fatalf("mkdir pitfalls: %v", err)
	}

	// Detail A is extractable: ExtractDescriptivePitfall recognizes the K8s
	// minor-version + auto_upgrade pattern when it sees both phrases plus
	// a scaleway_ resource name in the text.
	detailA := `exit status 1 | stderr: minor version 1.31 must only be used with auto upgrade enabled, on resource scaleway_k8s_cluster.main`
	// Detail B is *generic* and intentionally not extractable. The oscillation
	// detector still surfaces it as oscillating, but ExtractDescriptivePitfall
	// returns nil for it, exercising the "skip non-extractable" branch in
	// run_command.go without polluting the learned pitfall file.
	detailB := `exit status 1 | stderr: validation failed`

	stageErrA := &harness.StageError{
		StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: detailA},
		Err:         errors.New(detailA),
	}
	stageErrB := &harness.StageError{
		StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: detailB},
		Err:         errors.New(detailB),
	}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 4
		cfg.Paths.Pitfalls = pitfallsDir
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:     &alternatingStaticHarness{errs: [2]error{stageErrA, stageErrB}},
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure when budget is exhausted")
	}

	if !strings.Contains(stdout.String(), "run/terminal_reason: pass (repair_budget_exhausted)") {
		t.Fatalf("expected repair_budget_exhausted terminal reason, got:\n%s", stdout.String())
	}

	// Scenario default cloud is scaleway, so pitfalls file lands at
	// pitfalls/scaleway.yaml. The K8s detail (errA) should produce one
	// learned pitfall; errB is generic and should be skipped.
	pitfallsPath := filepath.Join(pitfallsDir, "scaleway.yaml")
	data, err := os.ReadFile(pitfallsPath)
	if err != nil {
		t.Fatalf("expected learned pitfalls file at %s: %v", pitfallsPath, err)
	}
	contents := string(data)
	for _, want := range []string{
		"resource: scaleway_k8s_cluster",
		"source: descriptive",
		"discovered_from: example-scenario",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("expected learned pitfalls to contain %q, got:\n%s", want, contents)
		}
	}
	// The generic errB rule must NOT have been written.
	if strings.Contains(contents, "validation failed") {
		t.Fatalf("expected non-extractable generic failure to be skipped, got:\n%s", contents)
	}
}

// sequencedStaticHarness returns errors in order from a fixed slice,
// looping back to start once exhausted. Used to drive run_command
// through specific multi-iteration failure sequences (not just a
// two-way alternation) — the [A, B, C, A, D] case the new
// all-iterations scan must catch.
type sequencedStaticHarness struct {
	calls int
	errs  []error
}

func (s *sequencedStaticHarness) Run(ctx context.Context, workDir string, env map[string]string) (*harness.StaticResult, error) {
	idx := s.calls % len(s.errs)
	s.calls++
	return nil, s.errs[idx]
}

// TestRunCommandLearnsRecurringPitfallWhenLastIterationDiffers pins the
// fix for the 20260529T095429Z bug: the LLM's mistake (`private_ip`
// instead of `private_ips`) recurred in iters 1 and 4, but iter 5
// finally compiled and failed on a downstream connectivity check.
// The prior "last iteration only" fallback inspected iter 5's
// connectivity failure (unlearnable — no resource named) and missed
// the actual recurring lesson. With the broadened all-iterations
// scan + NormalizeDetail-based dedup, the recurring `private_ip`
// mistake from iters 1/4 must produce a learned pitfall even when
// the last iteration's failure is unrelated.
func TestRunCommandLearnsRecurringPitfallWhenLastIterationDiffers(t *testing.T) {
	h := newCommandTestHarness(t)

	pitfallsDir := filepath.Join(h.WorkspaceDir, "pitfalls")
	if err := os.MkdirAll(pitfallsDir, 0o755); err != nil {
		t.Fatalf("mkdir pitfalls: %v", err)
	}

	// Iter 1 / iter 4 (A): the recurring Unsupported-attribute mistake
	// on scaleway_instance_server.<instance>.private_ip. Slightly
	// different shapes — `web[*]` vs `web_0` — to verify NormalizeDetail
	// collapses them into one dedup bucket.
	detailA1 := `exit status 1 | stderr: Error: Unsupported attribute on loadbalancer.tf line 21, scaleway_instance_server.web[*].private_ip — This object does not have an attribute named "private_ip".`
	detailA2 := `exit status 1 | stderr: Error: Unsupported attribute on loadbalancer.tf line 23, scaleway_instance_server.web_0.private_ip — This object has no argument, nested block, or exported attribute named "private_ip". Did you mean "private_ips"?`
	// Iter 2 (B): public_ip — distinct mistake, learnable on its own
	detailB := `exit status 1 | stderr: Error: Unsupported attribute on loadbalancer.tf line 24, scaleway_instance_server.web[*].public_ip — This object does not have an attribute named "public_ip".`
	// Iter 3 (C): a different shape — vpc policy fail
	detailC := `scaleway_instance_server.web[0] is not attached to a private network via scaleway_instance_private_nic`
	// Iter 5 (D): connectivity test failure with NO resource name —
	// not learnable. This is what the prior code inspected exclusively.
	detailD := `connectivity "public_internet->database:5432" expected false got true`

	mkErr := func(s string) error {
		return &harness.StageError{
			StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: s},
			Err:         errors.New(s),
		}
	}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 5
		cfg.Paths.Pitfalls = pitfallsDir
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static: &sequencedStaticHarness{errs: []error{
			mkErr(detailA1),
			mkErr(detailB),
			mkErr(detailC),
			mkErr(detailA2),
			mkErr(detailD),
		}},
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected run failure when budget is exhausted")
	}
	if !strings.Contains(stdout.String(), "repair_budget_exhausted") {
		t.Fatalf("expected repair_budget_exhausted terminal reason, got:\n%s", stdout.String())
	}

	pitfallsPath := filepath.Join(pitfallsDir, "scaleway.yaml")
	data, err := os.ReadFile(pitfallsPath)
	if err != nil {
		t.Fatalf("expected learned pitfalls file at %s: %v", pitfallsPath, err)
	}
	contents := string(data)
	// The recurring `private_ip` lesson must land in the file — this is
	// the regression that motivated the broader scan.
	if !strings.Contains(contents, "scaleway_instance_server") {
		t.Fatalf("expected pitfalls to learn scaleway_instance_server from iters 1/4, got:\n%s", contents)
	}
	if !strings.Contains(contents, "private_ip") {
		t.Fatalf("expected the bad attribute name in the rule body, got:\n%s", contents)
	}
	// The "Did you mean private_ips" suggestion (iter 4) must be forwarded
	// into the rule so the LLM sees the canonical fix.
	if !strings.Contains(contents, "private_ips") {
		t.Fatalf("expected the suggested attribute to be carried into the rule, got:\n%s", contents)
	}
	// Connectivity (iter 5) is unlearnable — must NOT pollute the file.
	if strings.Contains(contents, "public_internet") {
		t.Fatalf("unlearnable connectivity detail must not appear in pitfalls, got:\n%s", contents)
	}
}

// TestRunCommandLearnsAcrossClouds — the same all-iterations scan +
// Unsupported-attribute template must work uniformly for AWS and GCP.
// Earlier the cross-cloud guard silently dropped multi-cloud
// learnings; this test pins the contract that GCP and AWS scenarios
// each grow their own pitfalls/<cloud>.yaml from a same-shape failure.
func TestRunCommandLearnsAcrossClouds(t *testing.T) {
	for _, cloud := range []string{"gcp", "aws"} {
		cloud := cloud
		t.Run(cloud, func(t *testing.T) {
			h := newCommandTestHarness(t)
			// Override scenario cloud — default fixture is scaleway.
			scenarioYAML := `scenario: example-scenario
version: "1.0"
cloud: ` + cloud + `
description: example
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`
			if err := os.WriteFile(h.ScenarioPath, []byte(scenarioYAML), 0o644); err != nil {
				t.Fatalf("rewrite scenario: %v", err)
			}

			pitfallsDir := filepath.Join(h.WorkspaceDir, "pitfalls")
			if err := os.MkdirAll(pitfallsDir, 0o755); err != nil {
				t.Fatalf("mkdir pitfalls: %v", err)
			}

			// Mirror the user's bug shape against each cloud's
			// resource type so the cross-cloud guard accepts it.
			resourceType := map[string]string{
				"gcp": "google_compute_instance",
				"aws": "aws_instance",
			}[cloud]
			detail := `exit status 1 | stderr: Error: Unsupported attribute on main.tf line 14, ` + resourceType + `.web.bogus_attr — This object has no argument, nested block, or exported attribute named "bogus_attr". Did you mean "id"?`

			stageErr := &harness.StageError{
				StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: detail},
				Err:         errors.New(detail),
			}

			opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
				cfg.Agent.RepairIterationsMax = 4
				cfg.Paths.Pitfalls = pitfallsDir
				return cfg
			})
			opts.deps = RuntimeDependencies{
				Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
					return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
				}),
				Static:     &fakeStaticHarness{err: stageErr},
				MockDeploy: &fakeMockDeployHarness{},
				Destroy:    &fakeDestroyHarness{},
			}

			cmd := newRunCommandForTest(opts)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

			if err := cmd.Execute(); err == nil {
				t.Fatal("expected run failure")
			}

			pitfallsPath := filepath.Join(pitfallsDir, cloud+".yaml")
			data, err := os.ReadFile(pitfallsPath)
			if err != nil {
				t.Fatalf("expected learned pitfalls file for %s at %s: %v", cloud, pitfallsPath, err)
			}
			contents := string(data)
			if !strings.Contains(contents, resourceType) {
				t.Fatalf("%s pitfall missing resource %q, got:\n%s", cloud, resourceType, contents)
			}
			if !strings.Contains(contents, "bogus_attr") {
				t.Fatalf("%s pitfall missing bad attribute, got:\n%s", cloud, contents)
			}
			if !strings.Contains(contents, `id`) {
				t.Fatalf("%s pitfall missing 'Did you mean' suggestion, got:\n%s", cloud, contents)
			}
		})
	}
}

// TestRunCommandSkipsOscillationLearningWhenNoOscillation ensures the
// oscillation pitfall path stays inert when failures are sustained
// (linear) — only target_reached/iter-N->N-1 path should write learned
// pitfalls in that case, and that path is unrelated to oscillation.
// TestRunCommandLearnsFromStuckRepeatedSignature pins the M90 contract.
// Previously, when stuck-detection fired at 2 iterations with the SAME
// failure signature both times, no learning happened: DetectOscillation
// requires >= 3 iterations + a toggle pattern, so the most common
// failure mode ("LLM made the same mistake twice in a row") produced
// zero learned pitfalls. M90 added a second learning path that
// extracts from the repeated signature directly. AWS scenarios in
// M88's sweep hit this mode universally — without M90 the
// pitfalls/aws.yaml file could never grow from its own runs.
func TestRunCommandLearnsFromStuckRepeatedSignature(t *testing.T) {
	h := newCommandTestHarness(t)

	pitfallsDir := filepath.Join(h.WorkspaceDir, "pitfalls")
	if err := os.MkdirAll(pitfallsDir, 0o755); err != nil {
		t.Fatalf("mkdir pitfalls: %v", err)
	}

	detailA := `exit status 1 | stderr: minor version 1.31 must only be used with auto upgrade enabled, on resource scaleway_k8s_cluster.main`
	stageErr := &harness.StageError{
		StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: detailA},
		Err:         errors.New(detailA),
	}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 4
		cfg.Paths.Pitfalls = pitfallsDir
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:     &fakeStaticHarness{err: stageErr},
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected run failure")
	}

	if !strings.Contains(stdout.String(), "run/terminal_reason: pass (stuck)") {
		t.Fatalf("expected stuck terminal reason for sustained failures, got:\n%s", stdout.String())
	}

	pitfallsPath := filepath.Join(pitfallsDir, "scaleway.yaml")
	data, err := os.ReadFile(pitfallsPath)
	if err != nil {
		t.Fatalf("M90 contract: expected pitfalls/scaleway.yaml to be written from the repeated-signature learning path, got: %v", err)
	}
	contents := string(data)
	if !strings.Contains(contents, "source: descriptive") {
		t.Fatalf("expected source: descriptive entry in pitfalls file, got:\n%s", contents)
	}
	if !strings.Contains(contents, "scaleway_k8s_cluster") {
		t.Fatalf("expected scaleway_k8s_cluster pitfall extracted from repeated signature, got:\n%s", contents)
	}
}
