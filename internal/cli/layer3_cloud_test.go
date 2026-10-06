package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

func TestParseLayer3Cloud(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want layer3Cloud
	}{
		{raw: "", want: layer3Scaleway},
		{raw: "scaleway", want: layer3Scaleway},
		{raw: "aws", want: layer3AWS},
	} {
		got, err := parseLayer3Cloud(tc.raw)
		require.NoError(t, err, "raw %q", tc.raw)
		assert.Equal(t, tc.want, got, "raw %q", tc.raw)
	}

	for _, raw := range []string{"gcp", "genesys", "azure"} {
		_, err := parseLayer3Cloud(raw)
		require.Error(t, err, "raw %q", raw)
		assert.Contains(t, err.Error(), raw)
	}
}

// recordingRunProject counts every call and succeeds at none, so a path
// that reaches the Account API at all is visible.
type recordingRunProject struct{ calls int }

func (r *recordingRunProject) Create(context.Context, string, string, string, string) (harness.RunProject, error) {
	r.calls++
	return harness.RunProject{}, errors.New("recorded")
}

func (r *recordingRunProject) Describe(context.Context, string, string) (harness.ProjectProvenance, error) {
	r.calls++
	return harness.ProjectProvenance{}, errors.New("recorded")
}

func (r *recordingRunProject) Delete(context.Context, string, string) error {
	r.calls++
	return errors.New("recorded")
}

func (r *recordingRunProject) List(context.Context, string, string) ([]harness.ListedProject, error) {
	r.calls++
	return nil, errors.New("recorded")
}

// layer3Fakes is every dependency that reaches a real cloud.
type layer3Fakes struct {
	runProject *recordingRunProject
	deploy     *fakeSandboxDeployHarness
	destroy    *fakeSandboxDestroyHarness
	sweep      *fakeOrphanSweep
	purge      *fakePurge
}

func newLayer3Fakes() layer3Fakes {
	return layer3Fakes{
		runProject: &recordingRunProject{},
		deploy:     &fakeSandboxDeployHarness{},
		destroy:    &fakeSandboxDestroyHarness{},
		sweep:      &fakeOrphanSweep{},
		purge:      &fakePurge{},
	}
}

func (f layer3Fakes) install(deps *RuntimeDependencies) {
	deps.RunProject = f.runProject
	deps.SandboxDeploy = f.deploy
	deps.SandboxDestroy = f.destroy
	deps.OrphanSweep = f.sweep
	deps.AutoCreated = f.purge
}

func (f layer3Fakes) assertUntouched(t *testing.T) {
	t.Helper()
	assert.Zero(t, f.runProject.calls, "RunProject")
	assert.Zero(t, f.deploy.calls, "SandboxDeploy")
	assert.Zero(t, f.destroy.calls+f.destroy.withoutConfigCalls, "SandboxDestroy")
	assert.Zero(t, f.sweep.calls, "OrphanSweep")
	assert.Zero(t, f.purge.calls, "AutoCreated")
}

func withCloud(t *testing.T, yaml, cloud string) string {
	t.Helper()
	require.Contains(t, yaml, "cloud: scaleway\n")
	return strings.Replace(yaml, "cloud: scaleway\n", "cloud: "+cloud+"\n", 1)
}

func TestTestCommandRefusesANonScalewayCloudAtTheGate(t *testing.T) {
	for _, cloud := range []string{"aws", "gcp"} {
		t.Run(cloud, func(t *testing.T) {
			h := newCommandTestHarness(t)
			sandboxCredsForTest(t)
			raw, err := os.ReadFile(h.ScenarioPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(h.ScenarioPath, []byte(withCloud(t, string(raw), cloud)), 0o600))

			// The premise: Scaleway's gate would pass this stack, so only
			// the cloud can refuse it.
			cfg, err := config.Load(h.ConfigPath)
			require.NoError(t, err)
			require.NoError(t, validateLayer3HCLShape(filepath.Join(h.OutputDir(), "example-scenario"),
				cfg.Validation.Layers.SandboxDeploy.AllowResourceTypes))

			fakes := newLayer3Fakes()
			mock := &fakeMockDeployHarness{result: &harness.MockDeployResult{StateSnapshot: []byte(`{}`)}}
			opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
				cfg.Validation.Layers.SandboxDeploy.Enabled = true
				return cfg
			})
			opts.deps = RuntimeDependencies{MockDeploy: mock, Destroy: &fakeDestroyHarness{}}
			fakes.install(&opts.deps)

			cmd := newTestCommandForTest(opts)
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)})
			require.Error(t, cmd.Execute())

			result := decodeMachineOutput(t, stdout)
			require.Len(t, result.Failures, 1)
			assert.Equal(t, "sandbox_deploy", result.Failures[0].Layer)
			assert.Equal(t, "allowlist", result.Failures[0].Stage)
			assert.Contains(t, result.Failures[0].Detail, cloud)

			assert.Zero(t, mock.calls, "refused before any tofu")
			fakes.assertUntouched(t)
		})
	}
}

func TestGenerationGateIsKeyedOnTheScenarioCloud(t *testing.T) {
	stack := map[string][]byte{"main.tf": []byte(shapeProject)}
	for _, tc := range []struct {
		name, payload, ami, want string
		notGenerated             bool
	}{
		{name: "scaleway passes", payload: "cloud: scaleway\n"},
		// With its AMI resolved, aws reaches the AWS gate, which refuses a
		// Scaleway stack.
		{name: "aws is refused", payload: "cloud: aws\n", ami: "ami-0deadbeef1234567", want: `resource type "scaleway_block_volume" is not a aws_* type`},
		{name: "aws without a resolved AMI", payload: "cloud: aws\n", want: "no resolved AMI id", notGenerated: true},
		{name: "unreadable cloud is refused", payload: "cloud: {nested: true}\n", want: "read cloud"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scenarioPath := filepath.Join(t.TempDir(), "scenario.yaml")
			require.NoError(t, os.WriteFile(scenarioPath, []byte("scenario: gate\n"+tc.payload), 0o600))
			cfg := config.Default()
			cfg.Validation.Layers.SandboxDeploy.Enabled = true
			var calls []generator.Request
			rt := &CommandRuntime{
				Config:       cfg,
				outputDir:    t.TempDir(),
				AWSLayer3AMI: tc.ami,
				Deps: RuntimeDependencies{Generator: generator.SeedGeneratorFunc(
					func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
						calls = append(calls, req)
						return &generator.GeneratedCode{Files: stack}, nil
					})},
			}

			_, _, err := generateAndWriteFilesWithResult(context.Background(), rt, scenarioPath, "", 1, nil, generatedFileWriteModeClean)
			if tc.notGenerated {
				assert.Empty(t, calls, "the generator was called")
			} else if assert.Len(t, calls, 1) {
				assert.Equal(t, tc.ami, calls[0].AMIID)
			}
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestGenerateHandsAWSTheLayer2AMI(t *testing.T) {
	h := newUserDataHarness(t, awsInstanceScenarioPath, map[string][]byte{"main.tf": []byte(awsInstanceHCL)})

	require.NoError(t, h.generate(generatedFileWriteModeClean))

	assert.Equal(t, "ami-0al2023x8664", h.req.AMIID)
	assert.Equal(t, harness.AWSLayer2AMI, h.req.AMIID)
}

func TestRunKeepRefusesANonScalewayCloudBeforeCredentials(t *testing.T) {
	for _, tc := range []struct{ cloud, want string }{
		{cloud: "aws", want: "live path"},
		{cloud: "gcp", want: "no Layer 3 support"},
	} {
		t.Run(tc.cloud, func(t *testing.T) {
			h := newCommandTestHarness(t)
			sandboxCredsForTest(t)
			scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "web-live-paris.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(scenarioPath), 0o755))
			require.NoError(t, os.WriteFile(scenarioPath, []byte(withCloud(t, liveServiceScenarioYAML, tc.cloud)), 0o600))

			generated := 0
			fakes := newLayer3Fakes()
			opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
				cfg.Validation.Layers.SandboxDeploy.Enabled = true
				return cfg
			})
			opts.deps = RuntimeDependencies{
				Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
					generated++
					return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
				}),
				Static:     &fakeStaticHarness{result: &harness.StaticResult{PlanJSON: []byte(`{}`)}},
				MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{StateSnapshot: []byte(`{}`)}},
				Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{StateSnapshot: []byte(`{}`)}},
			}
			fakes.install(&opts.deps)

			cmd := newRunCommandForTest(opts)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath, "--keep"})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.cloud)
			assert.Contains(t, err.Error(), tc.want)
			assert.Zero(t, generated, "refused before the LLM was asked for anything")
			fakes.assertUntouched(t)
		})
	}
}

func TestDeployRefusesANonScalewayCloudBeforeCredentials(t *testing.T) {
	for _, tc := range []struct{ cloud, want string }{
		{cloud: "aws", want: "live path"},
		{cloud: "gcp", want: "no Layer 3 support"},
	} {
		t.Run(tc.cloud, func(t *testing.T) {
			sandboxCredsForTest(t)
			fakes := newLayer3Fakes()
			rt, store, scenarioPath := deployTestRuntime(t, withCloud(t, liveServiceScenarioYAML, tc.cloud), fakes.deploy)
			fakes.install(&rt.Deps)
			writeDeployableHCL(t, rt.OutputDir())

			err := runDeploy(t, rt, scenarioPath, &strings.Builder{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.cloud)
			assert.Contains(t, err.Error(), tc.want)
			fakes.assertUntouched(t)

			deployments, _, listErr := store.List()
			require.NoError(t, listErr)
			assert.Empty(t, deployments, "nothing was recorded")
		})
	}
}

func TestLiveUpgradeTakesTheCloudFromTheRecord(t *testing.T) {
	for _, tc := range []struct{ cloud, refusal string }{
		{cloud: "aws", refusal: "live path"},
		{cloud: "gcp", refusal: "no Layer 3 support"},
		{cloud: ""},
		{cloud: "scaleway"},
	} {
		t.Run("cloud="+tc.cloud, func(t *testing.T) {
			sandboxCredsForTest(t)
			probe := &stagingVersionProbe{running: "nginx/1.27.4"}
			deploy := &fakeSandboxDeployHarness{}
			rt, store := upgradeRuntime(t, probe, deploy)
			d := upgradeableDeployment(t, store, "dep-cloud", "1.27")
			d.Cloud = tc.cloud
			require.NoError(t, store.Put(d))

			if tc.refusal == "" {
				deploy.onRun = func() { probe.running = "nginx/1.28.0" }
				require.NoError(t, runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28"))
				assert.Equal(t, 1, deploy.calls, "past the cloud check and applied")
				return
			}

			fakes := newLayer3Fakes()
			fakes.deploy = deploy
			fakes.install(&rt.Deps)
			err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.cloud)
			assert.Contains(t, err.Error(), tc.refusal)
			assert.Zero(t, probe.probes, "refused before the service was probed")
			fakes.assertUntouched(t)
		})
	}
}

// runLiveJSON runs a live subcommand with --output json and decodes what
// it reported.
func runLiveJSON(t *testing.T, rt *CommandRuntime, run func(*cobra.Command, []string, *CommandRuntime) error, args ...string) (OutputResult, error) {
	t.Helper()
	cmd := &cobra.Command{Use: "live"}
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("output", string(OutputModeJSON), "")
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(context.Background())
	err := run(cmd, args, rt)
	return decodeMachineOutput(t, stdout), err
}

// decodeMachineOutput reads the first JSON document, ignoring whatever
// cobra prints after it on failure.
func decodeMachineOutput(t *testing.T, stdout *bytes.Buffer) OutputResult {
	t.Helper()
	var out MachineOutput
	require.NoError(t, json.NewDecoder(stdout).Decode(&out))
	return out.Result
}

// Named without "Scaleway": t.TempDir() embeds the test name in the state
// path the detail quotes.
func TestLiveTeardownAndReapRefuseAnAWSRecord(t *testing.T) {
	for name, run := range map[string]func(*cobra.Command, []string, *CommandRuntime) error{
		"teardown": runLiveTeardownCommand,
		"reap":     runLiveReapCommand,
	} {
		t.Run(name, func(t *testing.T) {
			sandboxCredsForTest(t)
			fakes := newLayer3Fakes()
			rt, store, workspace := liveTeardownRuntime(t, fakes.destroy, fakes.sweep)
			fakes.install(&rt.Deps)
			d := liveDeploymentWithState(t, store, workspace, "dep-aws", -time.Minute)
			d.Cloud = "aws"
			require.NoError(t, store.Put(d))

			result, err := runLiveJSON(t, rt, run, d.ID)
			require.Error(t, err)
			require.Len(t, result.Failures, 1)
			detail := result.Failures[0].Detail
			assert.Equal(t, "reclaimable", result.Failures[0].Check)
			assert.Contains(t, detail, "aws")
			assert.Contains(t, detail, filepath.Join(d.WorkDir, harness.LiveStateFilename))
			assert.Contains(t, detail, "live forget "+d.ID)
			assert.NotContains(t, detail, "Scaleway")
			assert.NotContains(t, detail, "reap <")
			fakes.assertUntouched(t)

			got, getErr := store.Get(d.ID)
			require.NoError(t, getErr)
			assert.Equal(t, livestore.StateLive, got.State, "not released")

			// The escape hatch the refusal names has to accept it.
			_, forgetErr := runLiveJSON(t, rt, runLiveForgetCommand, d.ID)
			require.NoError(t, forgetErr)
		})
	}
}

// hardcodedLayer3CloudArgs lists every call argument in f that is a
// layer3Cloud constant or conversion rather than a parsed value.
func hardcodedLayer3CloudArgs(fset *token.FileSet, f *ast.File) []string {
	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			expr := ast.Unparen(arg)
			switch e := expr.(type) {
			case *ast.Ident:
				if e.Name != "layer3Scaleway" && e.Name != "layer3AWS" {
					continue
				}
			case *ast.CallExpr:
				if fn, ok := e.Fun.(*ast.Ident); !ok || fn.Name != "layer3Cloud" {
					continue
				}
			default:
				continue
			}
			found = append(found, fset.Position(arg.Pos()).String()+": "+types.ExprString(arg))
		}
		return true
	})
	return found
}

// Every seam takes the cloud its caller parsed. A constant at a call site
// is a seam pinned to one cloud whatever the scenario says.
func TestNoCallSitePassesAHardcodedLayer3Cloud(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)
		for _, site := range hardcodedLayer3CloudArgs(fset, f) {
			t.Errorf("%s: pass the cloud parsed from the scenario or record instead", site)
		}
	}
}

func TestHardcodedLayer3CloudArgsFindsEachForm(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", `package cli
func x(cloud layer3Cloud) {
	seam(layer3Scaleway)
	seam((layer3AWS))
	seam(layer3Cloud("gcp"))
	seam(cloud)
	_ = cloud == layer3AWS
}`, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"x.go:3:7: layer3Scaleway",
		"x.go:4:7: (layer3AWS)",
		"x.go:5:7: layer3Cloud(\"gcp\")",
	}, hardcodedLayer3CloudArgs(fset, f))
}
