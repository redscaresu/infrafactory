package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

const seedRefused = `{"status":"error","message":"refused by the test's fakeaws"}`

// layer2Ends is both ends of the real Layer 2 deploy: fakeaws's admin API
// and a tofu on PATH. Both write to one log, so it orders every reset,
// seed and apply. seedStatus is what /mock/images answers.
type layer2Ends struct {
	url        string
	seedStatus int

	mu    sync.Mutex
	path  string
	seeds []map[string]string
}

func startLayer2Ends(t *testing.T, seedStatus int) *layer2Ends {
	t.Helper()
	ends := &layer2Ends{seedStatus: seedStatus, path: filepath.Join(t.TempDir(), "layer2.log")}
	bin := t.TempDir()
	tofu := "#!/bin/sh\ncase \"$1\" in apply) echo apply >> '" + ends.path + "';; esac\nexit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "tofu"), []byte(tofu), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	srv := httptest.NewServer(http.HandlerFunc(ends.serve))
	t.Cleanup(srv.Close)
	ends.url = srv.URL
	return ends
}

func (e *layer2Ends) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/mock/reset":
		e.record("reset", nil)
	case "/mock/state":
		_, _ = w.Write([]byte(`{}`))
	case "/mock/images":
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e.record("seed "+body["ami_id"], body)
		w.WriteHeader(e.seedStatus)
		if e.seedStatus != http.StatusOK {
			_, _ = w.Write([]byte(seedRefused))
		}
	default:
		e.record("unexpected "+r.Method+" "+r.URL.Path, nil)
		http.NotFound(w, r)
	}
}

func (e *layer2Ends) record(event string, seed map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if seed != nil {
		e.seeds = append(e.seeds, seed)
	}
	f, err := os.OpenFile(e.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

func (e *layer2Ends) events(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(e.path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}

// runAWSOverLayer2 is runAWSRun with the resolve serving orderAMI and the
// real mock deploy and mock state router in place of the fakes, talking to
// ends.
func runAWSOverLayer2(t *testing.T, lc *awsLifecycle, ends *layer2Ends, o awsRunOptions) awsRun {
	t.Helper()
	lc.params[harness.AWSAL2023AMIParameter] = orderAMI
	lc.amiImage = strings.Replace(lc.amiImage, harness.AWSLayer2AMI, orderAMI, 1)
	customize := o.customize
	o.customize = func(cfg *config.Config) {
		cfg.Fakeaws.URL = ends.url
		if customize != nil {
			customize(cfg)
		}
	}
	o.deps = func(d *RuntimeDependencies) { d.MockDeploy, d.MockState = nil, nil }
	return runAWSRun(t, lc, o)
}

var seededOrderAMI = "seed " + orderAMI

// Each iteration's mock deploy resets fakeaws, which drops the seed, so
// each one seeds again after its reset and before its apply.
func TestAWSRunSeedsTheResolvedAMIAfterEachMockReset(t *testing.T) {
	lc := newAWSLifecycle(t)
	applyFails(lc)
	ends := startLayer2Ends(t, http.StatusOK)

	run := runAWSOverLayer2(t, lc, ends, awsRunOptions{repairs: 2})

	require.Error(t, run.err)
	assert.Equal(t, 2, run.generates, "two iterations")
	assert.Equal(t, []string{"reset", seededOrderAMI, "apply", "reset", seededOrderAMI, "apply"}, ends.events(t))
	require.NotEmpty(t, ends.seeds)
	assert.Equal(t, map[string]string{
		"ami_id": orderAMI, "name": awsSeededAMIName, "root_device_name": "/dev/xvda", "region": "eu-west-2",
	}, ends.seeds[0], "the resolved id and root device, in the Layer 2 region")
}

func TestAWSRunFailsMockDeployWhenTheSeedIsRefused(t *testing.T) {
	lc := newAWSLifecycle(t)
	ends := startLayer2Ends(t, http.StatusConflict)

	run := runAWSOverLayer2(t, lc, ends, awsRunOptions{repairs: 1})

	require.Error(t, run.err)
	assert.Equal(t, []string{"reset", seededOrderAMI}, ends.events(t), "no tofu apply")
	assert.Zero(t, lc.deploy.calls, "no real apply")
	assert.True(t, slices.ContainsFunc(run.result.Failures, func(f FailureSummary) bool {
		return f.Check == "seed" && strings.Contains(f.Detail, "seed fakeaws image "+orderAMI) &&
			strings.Contains(f.Detail, "refused by the test's fakeaws")
	}), "the mock deploy fails at its seed, naming it: %+v", run.result.Failures)
}

func TestAWSRunAtLayer2SeedsNothing(t *testing.T) {
	lc := newAWSLifecycle(t)
	ends := startLayer2Ends(t, http.StatusOK)

	runAWSOverLayer2(t, lc, ends, awsRunOptions{repairs: 1, customize: func(cfg *config.Config) {
		cfg.Validation.Layers.SandboxDeploy.Enabled = false
	}})

	assert.Equal(t, []string{"reset", "apply"}, ends.events(t))
	assert.Empty(t, ends.seeds)
}

func TestSeedAWSLayer2AMISeedsNothingForAnotherCloud(t *testing.T) {
	ends := startLayer2Ends(t, http.StatusOK)
	runtime := &CommandRuntime{
		Config:         config.Config{Fakeaws: config.FakeawsConfig{URL: ends.url}},
		AWSLayer3AMI:   orderAMI,
		loadedScenario: &scenario.Scenario{Cloud: "scaleway"},
	}

	require.NoError(t, runtime.seedAWSLayer2AMI(context.Background()))
	assert.Empty(t, ends.events(t))
}
