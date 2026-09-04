package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

func servingScenario() *scenario.Scenario {
	return &scenario.Scenario{
		Name: "lb-serving-paris", Cloud: "scaleway",
		Resources: scenario.Resources{
			Compute:    &scenario.ComputeResource{Purpose: "web-server", Size: "small"},
			Networking: &scenario.NetworkingResource{LoadBalancer: &scenario.LoadBalancer{Exposure: "public"}},
		},
		Service: &scenario.ServiceSpec{
			Image: "nginx", Tag: "1.27", Port: 80, HealthPath: "/", TTL: "4h",
		},
	}
}

// "4h" is a number people agree to without doing the arithmetic.
// "expires at 03:47" is one they check against whether they will still
// be awake.
func TestPreviewStatesExpiryAsWallClockNotJustADuration(t *testing.T) {
	now := time.Date(2026, 9, 2, 23, 47, 0, 0, time.UTC)

	got := previewFor(servingScenario(), "", now)

	require.True(t, got.Deployable, got.Reason)
	assert.Equal(t, "4h0m0s", got.TTL)
	require.NotNil(t, got.ExpiresAt)
	assert.Equal(t, now.Add(4*time.Hour), *got.ExpiresAt)
	assert.Contains(t, got.ExpiresLocal, "03:47", "the hour it actually ends")
}

// A confidently wrong number shown at the moment somebody decides to
// spend money is worse than an admitted estimate.
func TestPreviewAlwaysSaysTheCostIsAListPrice(t *testing.T) {
	got := previewFor(servingScenario(), "", time.Now())

	assert.Contains(t, got.CostSummary, "list price")
	assert.True(t, got.Cost.Complete)
	assert.InDelta(t, 0.042, got.Cost.EurPerHour, 0.0005)
}

func TestPreviewAdmitsWhenItCouldNotPriceEverything(t *testing.T) {
	sc := servingScenario()
	sc.Resources.Kubernetes = &scenario.KubernetesResource{}

	got := previewFor(sc, "", time.Now())

	assert.False(t, got.Cost.Complete)
	assert.Contains(t, got.CostSummary, "AT LEAST")
}

// A different question from what it costs, and the one people forget to
// ask.
func TestPreviewSaysWhetherItWillBeReachableFromTheInternet(t *testing.T) {
	assert.True(t, previewFor(servingScenario(), "", time.Now()).InternetFacing)

	// Neither a public load balancer NOR compute: removing only the
	// networking block leaves an instance that still gets a public
	// address, which is the case an earlier version of this test got
	// wrong.
	closed := servingScenario()
	closed.Resources.Networking = nil
	closed.Resources.Compute = nil
	closed.Resources.Storage = &scenario.StorageResource{}
	assert.False(t, previewFor(closed, "", time.Now()).InternetFacing)
}

// The same refusal runDeployCommand makes: without a versioned
// application, "deploy" would just mean "apply and forget to destroy".
func TestPreviewRefusesAScenarioWithNoServiceBlock(t *testing.T) {
	sc := servingScenario()
	sc.Service = nil

	got := previewFor(sc, "", time.Now())

	assert.False(t, got.Deployable)
	assert.Contains(t, got.Reason, "no service: block")
	// Still says what it would create, because that is what explains a
	// greyed-out button.
	assert.NotEmpty(t, got.Cost.Components)
}

// ADR-0024 has no unbounded form. A TTL that will not parse makes the
// scenario undeployable rather than defaulting to something -- guessing
// a lifetime is how a deployment outlives everyone's memory of it.
func TestPreviewRefusesRatherThanGuessingALifetime(t *testing.T) {
	got := previewFor(servingScenario(), "not-a-duration", time.Now())

	assert.False(t, got.Deployable)
	assert.Contains(t, got.Reason, "ttl")
	assert.Zero(t, got.TTLSeconds)
}

func TestPreviewHonoursATTLOverride(t *testing.T) {
	now := time.Now()
	got := previewFor(servingScenario(), "1h", now)

	require.True(t, got.Deployable, got.Reason)
	assert.Equal(t, int64(3600), got.TTLSeconds)
	assert.InDelta(t, 0.042, got.Cost.EurAtTTL(time.Hour), 0.001)
}

// A warning that fires on every load balancer is one people learn to
// skip, which costs more than the case it was meant to catch.
func TestPreviewDoesNotCallAPrivateLoadBalancerInternetFacing(t *testing.T) {
	sc := servingScenario()
	sc.Resources.Networking.LoadBalancer.Exposure = "private"
	// Compute removed on purpose: an instance gets its own public
	// address, so leaving it in would test the wrong thing.
	sc.Resources.Compute = nil

	assert.False(t, previewFor(sc, "", time.Now()).InternetFacing)
}

// findScenarioPathByName strips the extension and accepts BOTH .yaml and
// .yml, so appending .yaml would 500 on every .yml-backed scenario — a
// file the discovery walk deliberately supports.
func TestPreviewFindsAScenarioStoredAsYml(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`scenario: yml-scenario
version: "1.0"
cloud: scaleway
description: stored with the other spelling
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "yml-scenario.yml"), body, 0o644))

	cfg := config.Default()
	cfg.Paths.Scenarios = dir
	srv := NewServer(ServerConfig{Config: cfg})

	req := httptest.NewRequest(http.MethodGet, "/api/deployments/preview?scenario=yml-scenario", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "yml-scenario")
}

func TestPreviewIsAReadOnlyGet(t *testing.T) {
	srv := NewServer(ServerConfig{Config: config.Default()})
	for _, m := range []string{http.MethodPost, http.MethodDelete, http.MethodPut} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(m, "/api/deployments/preview?scenario=x", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, m)
	}
}

// The year-one class, again. S159a closed it for the deployments payload
// and it came straight back in this one, because that test asserted over
// ONE payload and this is a new one.
//
// Every response type this API adds needs the same assertion, and the
// undeployable case is where it bites: a greyed-out scenario never sets
// an expiry.
func TestPreviewNeverCarriesAYearOneExpiry(t *testing.T) {
	for name, sc := range map[string]*scenario.Scenario{
		"no service block": func() *scenario.Scenario {
			s := servingScenario()
			s.Service = nil
			return s
		}(),
		"unparseable ttl": servingScenario(),
	} {
		t.Run(name, func(t *testing.T) {
			ttl := ""
			if name == "unparseable ttl" {
				ttl = "not-a-duration"
			}
			payload, err := json.Marshal(previewFor(sc, ttl, time.Now()))
			require.NoError(t, err)
			assert.NotContains(t, string(payload), "0001-01-01",
				"a greyed-out scenario must not report expiring in the year 1")
		})
	}
}

// Telling a GCP user their deploy will create a "DEV1-S instance" names
// a resource that will not exist, which is worse than being vague: this
// endpoint's whole job is saying what deploy would do.
func TestPreviewDoesNotNameScalewayShapesForOtherClouds(t *testing.T) {
	sc := servingScenario()
	sc.Cloud = "gcp"

	got := previewFor(sc, "", time.Now())

	for _, c := range got.Cost.Components {
		assert.NotContains(t, c.Name, "DEV1-S")
		assert.NotContains(t, c.Name, "LB-S")
	}
	assert.NotEmpty(t, got.Cost.Components, "it must still say what will be created")
}

// Discovery matches on the scenario's NAME and returns an
// extension-less path, so two files sharing a stem could have this load
// the wrong one. The consequence is not a 500: it is a confirmation
// showing the cost, lifetime and blast radius of a DIFFERENT scenario,
// immediately before somebody agrees to spend money.
func TestPreviewNeverAnswersAboutADifferentScenario(t *testing.T) {
	dir := t.TempDir()
	write := func(file, name string) {
		body := []byte("scenario: " + name + `
version: "1.0"
cloud: scaleway
description: two files, one stem
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`)
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), body, 0o644))
	}
	// Same stem, different scenarios inside.
	write("shared-stem.yaml", "the-yaml-one")
	write("shared-stem.yml", "the-yml-one")

	cfg := config.Default()
	cfg.Paths.Scenarios = dir
	srv := NewServer(ServerConfig{Config: cfg})

	req := httptest.NewRequest(http.MethodGet, "/api/deployments/preview?scenario=the-yml-one", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "the-yml-one")
	assert.NotContains(t, rec.Body.String(), "the-yaml-one",
		"answering about the wrong scenario is worse than answering nothing")
}

// Understating exposure at the moment of the decision is the worse
// direction: a compute instance gets a public IPv4 whether or not there
// is a load balancer in front of it.
func TestPreviewCallsAComputeOnlyScenarioInternetFacing(t *testing.T) {
	sc := servingScenario()
	sc.Resources.Networking = nil

	got := previewFor(sc, "", time.Now())

	assert.True(t, got.InternetFacing, "the instance still gets a public address")
}

// The preview is meant to be everything a person needs in order to
// decide, and "this server will refuse" is part of that.
func TestPreviewReportsWhetherTheServerWouldAcceptTheDeploy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sc.yaml"), []byte(`scenario: previewable
version: "1.0"
cloud: scaleway
description: x
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`), 0o644))

	cfg := config.Default()
	cfg.Paths.Scenarios = dir

	for name, deployer := range map[string]DeploymentDeployer{
		"without the flag": nil,
		"with the flag":    &fakeDeployer{},
	} {
		t.Run(name, func(t *testing.T) {
			srv := NewServer(ServerConfig{Config: cfg, Deployer: deployer})
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec,
				httptest.NewRequest(http.MethodGet, "/api/deployments/preview?scenario=previewable", nil))

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var got deployPreview
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.Equal(t, deployer != nil, got.Allowed)
		})
	}
}

// The lock stops the accidental duplicate. It does nothing about
// deploy → wait → deploy again, which produces a second run-owned
// project just the same. A lock cannot tell "I forgot" from "I meant
// it", so the confirmation says what already exists and the reader
// decides.
func TestPreviewNamesDeploymentsOfThisScenarioThatAlreadyExist(t *testing.T) {
	live := livestore.Deployment{
		ID: "dep-existing", Scenario: "previewable", Cloud: "scaleway",
		ProjectID: "proj-1", State: livestore.StateLive,
		CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour),
	}
	released := live
	released.ID = "dep-gone"
	released.State = livestore.StateReleased
	other := live
	other.ID = "dep-other"
	other.Scenario = "something-else"

	got := previewWithEstate(t, []livestore.Deployment{live, released, other})

	assert.Equal(t, []string{"dep-existing"}, got.AlreadyLive,
		"a released deployment is not still running, and another scenario is not this one")
}

func TestPreviewReportsNoExistingDeploymentsAsAnEmptyList(t *testing.T) {
	got := previewWithEstate(t, nil)

	assert.NotNil(t, got.AlreadyLive, "null would read as unknown")
	assert.Empty(t, got.AlreadyLive)
}

func previewWithEstate(t *testing.T, deployments []livestore.Deployment) deployPreview {
	t.Helper()
	return previewOf(t, ServerConfig{Deployments: &fakeDeployments{deployments: deployments}})
}

// previewOf writes the one scenario every preview test previews, serves
// a GET for it, and returns the decoded body.
//
// `cfg.Config` and `cfg.Paths.Scenarios` are filled in here; everything
// else -- the lister, the deployer -- is the caller's, which is the only
// thing these tests actually vary. The scenario document had been
// copy-pasted into five of them, so a schema change meant editing five
// fixtures and CI reported five unrelated failures.
func previewOf(t *testing.T, sc ServerConfig) deployPreview {
	t.Helper()
	rec := previewResponse(t, sc)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got deployPreview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// previewResponse is previewOf without the status assertion, for the
// tests that are about the status.
func previewResponse(t *testing.T, sc ServerConfig) *httptest.ResponseRecorder {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sc.yaml"), []byte(`scenario: previewable
version: "1.0"
cloud: scaleway
description: x
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`), 0o644))

	cfg := config.Default()
	cfg.Paths.Scenarios = dir
	sc.Config = cfg

	rec := httptest.NewRecorder()
	NewServer(sc).Handler.ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/deployments/preview?scenario=previewable", nil))
	return rec
}

// The unreadable branch is the whole reason AlreadyLiveUnknown exists,
// and it had no server-side test: changing `len(unreadable) > 0` to
// `false` restored the exact false negative its docstring calls out,
// with the whole Go suite green.
func TestPreviewSaysWhenTheEstateCouldNotBeFullyRead(t *testing.T) {
	got := previewOf(t, ServerConfig{
		Deployments: &fakeDeployments{
			unreadable: []error{errors.New("dep-broken.json: unexpected end of JSON input")},
		},
	})

	assert.True(t, got.AlreadyLiveUnknown,
		"an undecodable record could be a deployment of THIS scenario and would never match")
}

// A server with no lister never looked, so it must not claim it did.
func TestPreviewWithNoListerDoesNotClaimNothingIsDeployed(t *testing.T) {
	got := previewOf(t, ServerConfig{})

	assert.True(t, got.AlreadyLiveUnknown)
	assert.Empty(t, got.AlreadyLive)
}

// A deploy that is APPLYING has no record yet -- registration runs after
// the apply returns -- so the estate cannot see the one case where the
// reader is most likely duplicating.
func TestPreviewWarnsWhenTheScenarioIsApplyingRightNow(t *testing.T) {
	// The estate is NOT empty. An empty lister would make the
	// `AlreadyLive` assertion below pass for the wrong reason -- there
	// was nothing to find -- and it would keep passing with the
	// in-flight warning removed entirely. A live record for a DIFFERENT
	// scenario proves the lister was consulted and answered, and that
	// the applying scenario is still absent from what it returned.
	got := previewOf(t, ServerConfig{
		Deployments: &fakeDeployments{deployments: []livestore.Deployment{
			{ID: "dep-other", Scenario: "something-else", State: livestore.StateLive},
		}},
		Deployer: &fakeDeployer{inFlight: []string{"previewable"}},
	})

	assert.True(t, got.AlreadyDeploying,
		"the estate cannot see an apply in progress; the in-flight list is the only thing that can")
	assert.False(t, got.AlreadyLiveUnknown, "the estate was read in full")
	assert.Empty(t, got.AlreadyLive,
		"a readable estate that holds another scenario's deployment, and no record of this one -- "+
			"registration runs after the apply returns")
}

// A blank scenario name never looked, so it must not claim it did.
func TestPreviewWithABlankScenarioNameMakesNoClaim(t *testing.T) {
	got, unknown := liveDeploymentsOf(&serverState{deployments: &fakeDeployments{}}, "")

	assert.Empty(t, got)
	assert.True(t, unknown, "returning (empty, false) is the claim the flag exists to forbid")
}

// orderRecordingDeployer notes when its in-flight list was read.
type orderRecordingDeployer struct {
	fakeDeployer
	read *bool
}

func (d *orderRecordingDeployer) InFlight() []string {
	*d.read = true
	return d.fakeDeployer.inFlight
}

// orderRecordingLister notes whether the in-flight list had already been
// read by the time the estate was.
type orderRecordingLister struct {
	inFlightRead     *bool
	sawInFlightFirst bool
}

func (l *orderRecordingLister) List() ([]livestore.Deployment, []error, error) {
	l.sawInFlightFirst = *l.inFlightRead
	return nil, nil, nil
}

// The two reads cover each other in ONE direction only, so their order
// is a guarantee rather than a style choice.
//
// A deploy has no record until registration, which runs after the apply
// returns. Read the estate first and a deploy that finishes in between
// is in neither answer -- not in the estate, because it had not
// registered; not in the in-flight list, because it had already
// released -- and the confirmation renders with no warning at all and
// an explicit "checked, and nothing exists" claim, at the exact moment
// the scenario went live.
//
// The other order has no such window: anything that leaves the
// in-flight list after the first read has registered before the second.
func TestThePreviewReadsWhatIsApplyingBeforeItReadsTheEstate(t *testing.T) {
	inFlightRead := false
	lister := &orderRecordingLister{inFlightRead: &inFlightRead}

	previewOf(t, ServerConfig{
		Deployments: lister,
		Deployer:    &orderRecordingDeployer{read: &inFlightRead},
	})

	assert.True(t, lister.sawInFlightFirst,
		"reading the estate first leaves a window where a deploy finishing in between "+
			"is invisible to both reads, and the confirmation claims nothing exists")
}
