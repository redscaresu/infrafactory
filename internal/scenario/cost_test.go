package scenario

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lbServingScenario() *Scenario {
	return &Scenario{
		Name: "lb-serving-paris", Cloud: "scaleway",
		Resources: Resources{
			Compute: &ComputeResource{Purpose: "web-server", Size: "small"},
			Networking: &NetworkingResource{
				LoadBalancer: &LoadBalancer{Exposure: "public"},
			},
		},
	}
}

// The one shape ADR-0024 priced by hand. If this drifts, the ADR and the
// code disagree about what the demo costs.
func TestEstimateMatchesTheShapeTheADRPriced(t *testing.T) {
	est := lbServingScenario().EstimateCost()

	require.True(t, est.Complete, "this shape is fully priced: %v", est.Unpriced)
	assert.InDelta(t, 0.042, est.EurPerHour, 0.0005,
		"ADR-0024 records €0.042/hour for lb-serving-paris")
	assert.InDelta(t, 0.17, est.EurAtTTL(4*time.Hour), 0.01)
}

// The load-bearing case. A total that quietly omits what it could not
// price is wrong in the REASSURING direction, and a confidently wrong
// number shown at the moment somebody decides to spend money is worse
// than no number.
func TestEstimateNamesWhatItCouldNotPrice(t *testing.T) {
	sc := lbServingScenario()
	sc.Resources.Database = &DatabaseResource{}
	sc.Resources.Kubernetes = &KubernetesResource{}

	est := sc.EstimateCost()

	assert.False(t, est.Complete)
	assert.Contains(t, est.Unpriced, "managed database")
	assert.Contains(t, est.Unpriced, "Kubernetes cluster")
	assert.Contains(t, est.Summary(4*time.Hour), "AT LEAST")
	assert.Contains(t, est.Summary(4*time.Hour), "no list price")
}

// Free and unknown are different facts, and summing them as if both were
// zero is how a total silently understates itself.
func TestUnpricedComponentsAreNotTreatedAsFree(t *testing.T) {
	sc := lbServingScenario()
	sc.Resources.Kubernetes = &KubernetesResource{}

	est := sc.EstimateCost()

	var k8s CostComponent
	for _, c := range est.Components {
		if c.Name == "Kubernetes cluster" {
			k8s = c
		}
	}
	require.Equal(t, "Kubernetes cluster", k8s.Name)
	assert.False(t, k8s.Priced, "unknown is not zero")
	assert.Zero(t, k8s.EurPerHour)
}

// An overridden offer is what will actually be billed, and this project
// has a price for exactly one offer. Naming the real one and marking it
// unpriced beats pricing it as a DEV1-S because that is the number to
// hand.
func TestAnOverriddenOfferIsNamedAndNotMispriced(t *testing.T) {
	sc := lbServingScenario()
	sc.Resources.Compute.Override.Offer = "GP1-XL"

	est := sc.EstimateCost()

	assert.False(t, est.Complete)
	assert.Contains(t, est.Unpriced, "GP1-XL instance")
}

// The prices are Scaleway's. Pricing a GCP scenario from them would be a
// fabrication.
func TestAnotherCloudIsNotPricedFromScalewaysList(t *testing.T) {
	sc := lbServingScenario()
	sc.Cloud = "gcp"

	est := sc.EstimateCost()

	assert.False(t, est.Complete)
	assert.Zero(t, est.EurPerHour)
	assert.NotEmpty(t, est.Components, "it must still say what will be created")
}

// A confirmation dialog whose list reshuffles between renders is one a
// person stops reading.
func TestEstimateIsDeterministicallyOrdered(t *testing.T) {
	sc := lbServingScenario()
	sc.Resources.Database = &DatabaseResource{}
	sc.Resources.Redis = &RedisResource{}
	sc.Resources.Storage = &StorageResource{}

	first := sc.EstimateCost()
	for i := 0; i < 20; i++ {
		got := sc.EstimateCost()
		assert.Equal(t, first.Components, got.Components)
		assert.Equal(t, first.Unpriced, got.Unpriced)
	}
}

// The summary always says "list price", because an estimate a person
// cannot tell from a quote is one they will later feel misled by.
func TestSummaryAlwaysAdmitsItIsAListPrice(t *testing.T) {
	assert.Contains(t, lbServingScenario().EstimateCost().Summary(time.Hour), "list price")
}

func TestScenarioWithNoResourcesIsNotReportedAsComplete(t *testing.T) {
	est := (&Scenario{Name: "empty", Cloud: "scaleway"}).EstimateCost()

	assert.False(t, est.Complete, "nothing priced is not the same as everything priced")
	assert.Empty(t, est.Components)
}

// `exposure` is exactly "whether the load balancer has a public IP", per
// the schema. Billing one for a private balancer inflates the estimate
// and lists a component the scenario says will not exist — which makes
// the whole component list something a reader learns to skim.
func TestPrivateLoadBalancerIsNotChargedAPublicAddress(t *testing.T) {
	sc := lbServingScenario()
	sc.Resources.Networking.LoadBalancer.Exposure = "private"
	sc.Resources.Compute = nil

	est := sc.EstimateCost()

	for _, c := range est.Components {
		assert.NotEqual(t, "public IPv4 address", c.Name,
			"the scenario says this balancer has no public IP")
	}
	assert.InDelta(t, 0.023, est.EurPerHour, 0.0005, "the balancer alone")
}

func TestPublicLoadBalancerIsStillChargedAPublicAddress(t *testing.T) {
	sc := lbServingScenario()
	sc.Resources.Compute = nil

	est := sc.EstimateCost()

	assert.InDelta(t, 0.028, est.EurPerHour, 0.0005, "balancer plus its public IPv4")
}

// The class, closed by construction rather than by list.
//
// The estimator enumerated eleven resource blocks by hand and missed
// `iam` — the one whose absence matters most, since an API key and a
// policy are exactly what a blast-radius preview exists to surface.
// Adding it back would have been another snapshot.
//
// This walks `Resources` by reflection and asserts that EVERY declared
// block contributes at least one named component. A resource type added
// to the scenario schema now fails this test until the preview knows
// how to name it, rather than silently vanishing from the confirmation
// somebody reads before spending money.
func TestEveryDeclaredResourceBlockAppearsInThePreview(t *testing.T) {
	resourcesType := reflect.TypeOf(Resources{})

	for i := 0; i < resourcesType.NumField(); i++ {
		field := resourcesType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			resources := reflect.New(resourcesType).Elem()
			target := resources.Field(i)
			require.Equal(t, reflect.Ptr, target.Kind(),
				"this test assumes every resource block is a pointer")
			target.Set(reflect.New(target.Type().Elem()))

			sc := &Scenario{
				Name:      "reflection",
				Cloud:     "scaleway",
				Resources: resources.Interface().(Resources),
			}

			est := sc.EstimateCost()
			assert.NotEmpty(t, est.Components,
				"resources.%s creates something, and a preview that omits it "+
					"understates the blast radius", field.Name)
		})
	}
}

// Free and unknown are different facts, and the whole design of this
// type rests on that. Routing a VPC through the unknown path made an
// otherwise complete estimate report "AT LEAST", which teaches a reader
// to ignore the words that mean something.
func TestFreeComponentsDoNotMakeAnEstimateLookIncomplete(t *testing.T) {
	sc := lbServingScenario()
	sc.Resources.Networking.VPC = true
	sc.Resources.Networking.PrivateNetwork = true

	est := sc.EstimateCost()

	assert.True(t, est.Complete, "unpriced: %v", est.Unpriced)
	assert.NotContains(t, est.Summary(time.Hour), "AT LEAST")

	var vpc CostComponent
	for _, c := range est.Components {
		if c.Name == "VPC" {
			vpc = c
		}
	}
	require.Equal(t, "VPC", vpc.Name, "it must still be listed: this is a blast radius, not just a bill")
	assert.True(t, vpc.Priced, "free is known")
	assert.Zero(t, vpc.EurPerHour)
}

// `cloud: genesys` scenarios declare routing/identity/architect/
// full_stack, none of which the Go Resources struct carries — so they
// arrive as an empty resource set. An empty component list would render
// as "creates nothing, costs nothing", which is the most reassuring
// possible way to be wrong on the screen somebody reads before agreeing
// to spend.
func TestAnUnmodelledCloudIsNotReportedAsFree(t *testing.T) {
	est := (&Scenario{Name: "genesys-basic-queue", Cloud: "genesys"}).EstimateCost()

	assert.False(t, est.Modelled)
	assert.False(t, est.Complete)
	assert.Contains(t, est.Summary(time.Hour), "not modelled")
	assert.Contains(t, est.Summary(time.Hour), "do not read this as free")
}

// A scenario that genuinely declares no resources is a real, harmless
// case and must not be confused with one whose resources cannot be seen.
func TestAScenarioWithGenuinelyNoResourcesIsStillModelled(t *testing.T) {
	est := (&Scenario{Name: "empty", Cloud: "scaleway"}).EstimateCost()

	assert.True(t, est.Modelled, "nothing to create is different from nothing I can see")
	assert.False(t, est.Complete)
}

// The bill and the warning are two answers to one question, and they
// disagreed: a compute-only scenario was charged for a public address
// and told it was not reachable from the internet.
//
// Derived rather than re-decided, so whatever adds a public address also
// raises the warning — including anything added later.
func TestInternetFacingFollowsWhateverBillsAPublicAddress(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*Scenario)
		want   bool
	}{
		"compute only":         {func(s *Scenario) { s.Resources.Networking = nil }, true},
		"public load balancer": {func(s *Scenario) { s.Resources.Compute = nil }, true},
		"private load balancer only": {func(s *Scenario) {
			s.Resources.Compute = nil
			s.Resources.Networking.LoadBalancer.Exposure = "private"
		}, false},
		"neither": {func(s *Scenario) {
			s.Resources.Compute = nil
			s.Resources.Networking = nil
			s.Resources.Storage = &StorageResource{}
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			sc := lbServingScenario()
			tc.mutate(sc)
			est := sc.EstimateCost()

			billed := false
			for _, c := range est.Components {
				if c.Name == PublicAddressComponent {
					billed = true
				}
			}
			assert.Equal(t, tc.want, est.InternetFacing())
			assert.Equal(t, billed, est.InternetFacing(),
				"the bill and the warning must never disagree")
		})
	}
}
