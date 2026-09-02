package scenario

import (
	"fmt"
	"sort"
	"time"
)

// CostComponent is one billable thing a scenario will create, with the
// list price it was estimated at.
type CostComponent struct {
	// Name is the shape as a person would recognise it on an invoice.
	Name string `json:"name"`

	// Count is how many of it.
	Count int `json:"count"`

	// EurPerHour is the LIST price per unit, or zero when unpriced.
	EurPerHour float64 `json:"eur_per_hour"`

	// Priced is false when this project has no list price for the shape.
	//
	// A separate field rather than "EurPerHour == 0", because free and
	// unknown are different facts and summing them as if both were zero
	// is how a total silently understates itself.
	Priced bool `json:"priced"`

	// Source is where the price came from, so a reader can check it.
	Source string `json:"source,omitempty"`
}

// CostEstimate is what a scenario will cost if deployed, as far as this
// project actually knows.
//
// # Why `Unpriced` is a first-class field
//
// The prices below were read off Scaleway's pricing pages by hand for
// ONE shape (ADR-0024). Any scenario declaring something else -- a
// database, a Kubernetes cluster, object storage -- has components this
// project has never priced.
//
// A total that quietly omitted them would be **wrong in the reassuring
// direction**, and a confidently wrong number shown at the moment
// somebody decides to spend money is worse than no number at all. So
// unpriced components are counted, named, and the estimate says plainly
// that it is a floor rather than a total.
type CostEstimate struct {
	Components []CostComponent `json:"components"`

	// EurPerHour totals only the PRICED components.
	EurPerHour float64 `json:"eur_per_hour"`

	// Unpriced names the shapes with no list price here.
	Unpriced []string `json:"unpriced"`

	// Complete is true only when every component was priced. When it is
	// false, EurPerHour is a lower bound and must be presented as one.
	Complete bool `json:"complete"`

	// Modelled is false when this estimator has no model for the
	// scenario's resource shape at all.
	//
	// `cloud: genesys` scenarios declare `routing`, `identity`,
	// `architect` and `full_stack`, which the Go Resources struct does
	// not carry -- so they arrive here as an empty resource set. An
	// empty component list would then render as "this creates nothing
	// and costs nothing", which is the most reassuring possible way to
	// be wrong on the screen somebody reads before agreeing to spend.
	Modelled bool `json:"modelled"`
}

// EurAtTTL is the estimated cost of holding this shape for a duration.
func (c CostEstimate) EurAtTTL(ttl time.Duration) float64 {
	return c.EurPerHour * ttl.Hours()
}

// Summary states the estimate and its own limits in one line.
//
// It always says "list price" and it always says when it is incomplete,
// because both are the difference between an estimate a person can act
// on and a number they will later feel misled by.
func (c CostEstimate) Summary(ttl time.Duration) string {
	if !c.Modelled {
		return "this scenario's resources are not modelled here, so what it creates and " +
			"what it costs are both unknown — do not read this as free"
	}

	base := fmt.Sprintf("about €%.2f/hour at list price, €%.2f for %s",
		c.EurPerHour, c.EurAtTTL(ttl), ttl)
	if c.Complete {
		return base
	}
	return fmt.Sprintf("%s — AT LEAST, because %d component(s) have no list price here: %v",
		base, len(c.Unpriced), c.Unpriced)
}

// PublicAddressComponent is the component name for a public IPv4.
//
// Named because two things must agree about it: the cost estimate that
// bills it, and the confirmation that warns the shape is reachable from
// the internet. Those were separate hand-written conditions and they
// disagreed -- a compute-only scenario was billed a public address while
// the confirmation said it was not internet-facing, understating
// exposure at the moment of the decision.
const PublicAddressComponent = "public IPv4 address"

// InternetFacing reports whether this estimate includes anything with a
// public address.
//
// DERIVED from the component list rather than re-deciding from the
// scenario, so the warning and the bill cannot disagree. Whatever adds a
// public address also raises the warning, including anything added
// later.
func (c CostEstimate) InternetFacing() bool {
	for _, component := range c.Components {
		if component.Name == PublicAddressComponent && component.Count > 0 {
			return true
		}
	}
	return false
}

// scalewayListPrices are the per-hour figures ADR-0024 recorded, with
// their sources. Deliberately small: this is what has actually been
// looked up, not a guess at a catalogue.
var scalewayListPrices = map[string]struct {
	eurPerHour float64
	source     string
}{
	"DEV1-S instance":     {0.00898, "scaleway.com/en/pricing/virtual-instances"},
	"LB-S load balancer":  {0.023, "scaleway.com/en/pricing/network"},
	"public IPv4 address": {0.005, "scaleway.com/en/pricing/network"},
}

// hasUnmodelledResources reports whether a cloud declares resource
// blocks the Go Resources struct does not carry.
//
// `cloud: genesys` scenarios use `routing`, `identity`, `architect` and
// `full_stack` (scenario.schema.json), none of which have a field here,
// so they unmarshal into an empty resource set. Naming that explicitly
// beats inferring it from emptiness, because a scenario with genuinely
// no resources is a different and harmless thing.
func hasUnmodelledResources(cloud string) bool {
	return cloud == "genesys"
}

// defaultComputeName is what a scenario's compute block will become.
//
// Only Scaleway gets a concrete offer name, because only Scaleway's
// generator maps `size` onto one this project can name. Elsewhere the
// honest answer is the shape, not a guess at the SKU.
func defaultComputeName(cloud string) string {
	if cloud == "scaleway" {
		return "DEV1-S"
	}
	return "compute"
}

func defaultLoadBalancerName(cloud string) string {
	if cloud == "scaleway" {
		return "LB-S load balancer"
	}
	return "load balancer"
}

// EstimateCost prices what a scenario will create.
//
// Scaleway only: the prices are Scaleway's, and pretending to price a
// GCP or AWS scenario from them would be a fabrication. Another cloud
// returns an estimate with everything unpriced, which is honest and
// still tells a reader what will be created.
func (s *Scenario) EstimateCost() CostEstimate {
	est := CostEstimate{Components: []CostComponent{}, Unpriced: []string{}}

	// addFree records something that IS created and costs nothing.
	//
	// Distinct from the unpriced path, and that distinction is the whole
	// design of this type: free and unknown are different facts. Routing
	// a VPC through the unknown path would make an otherwise complete
	// estimate report "AT LEAST", teaching a reader to ignore the words
	// that mean something.
	addFree := func(name string, count int) {
		if count <= 0 {
			return
		}
		est.Components = append(est.Components, CostComponent{
			Name: name, Count: count, Priced: true, Source: "no charge",
		})
	}

	add := func(name string, count int) {
		if count <= 0 {
			return
		}
		price, known := scalewayListPrices[name]
		if s.Cloud != "scaleway" {
			known = false
		}
		component := CostComponent{Name: name, Count: count}
		if known {
			component.EurPerHour = price.eurPerHour
			component.Priced = true
			component.Source = price.source
			est.EurPerHour += price.eurPerHour * float64(count)
		} else {
			est.Unpriced = append(est.Unpriced, name)
		}
		est.Components = append(est.Components, component)
	}

	r := s.Resources

	if r.Compute != nil {
		count := r.Compute.Count
		if count == 0 {
			count = 1
		}
		// The offer, when overridden, is what will actually be billed --
		// and this project has a price for exactly one of them. Naming
		// the real offer and marking it unpriced beats pricing it as a
		// DEV1-S because that is the only number to hand.
		//
		// The DEFAULT is Scaleway-specific, so it is only assumed for a
		// Scaleway scenario. Telling a GCP user their deploy will create
		// a "DEV1-S instance" names a resource that will not exist,
		// which is worse than being vague: this endpoint's whole job is
		// saying what deploy would do.
		offer := r.Compute.Override.Offer
		if offer == "" {
			offer = defaultComputeName(s.Cloud)
		}
		add(offer+" instance", count)
		add(PublicAddressComponent, count)
	}

	if r.Networking != nil {
		// A VPC or private network is not billable and IS created, and
		// this list is a blast-radius preview before it is a bill. A
		// component that costs nothing still belongs in "what will this
		// make".
		if r.Networking.VPC {
			addFree("VPC", 1)
		}
		if r.Networking.PrivateNetwork {
			addFree("private network", 1)
		}
		if r.Networking.LoadBalancer == nil && !r.Networking.VPC && !r.Networking.PrivateNetwork {
			addFree("networking", 1)
		}
	}

	if r.Networking != nil && r.Networking.LoadBalancer != nil {
		add(defaultLoadBalancerName(s.Cloud), 1)
		// Only a PUBLIC load balancer gets a public IP -- that is what
		// the schema says `exposure` means. Billing one for a private
		// balancer inflates the estimate and, worse, lists a component
		// the scenario says will not exist, which makes the whole
		// component list something a reader learns to skim.
		if r.Networking.LoadBalancer.Public() {
			add(PublicAddressComponent, 1)
		}
	}

	for name, present := range map[string]bool{
		"IAM application and keys": r.IAM != nil,
		"managed database":         r.Database != nil,
		"Kubernetes cluster":       r.Kubernetes != nil,
		"Redis cluster":            r.Redis != nil,
		"object storage":           r.Storage != nil,
		"container registry":       r.Registry != nil,
		"managed DNS zone":         r.DNS != nil,
		"Cloud Run service":        r.CloudRun != nil,
		"secret manager":           r.SecretManager != nil,
		"pub/sub topic":            r.PubSub != nil,
		"DynamoDB table":           r.DynamoDB != nil,
		"messaging queue":          r.Messaging != nil,
	} {
		if present {
			add(name, 1)
		}
	}

	// Deterministic order: this feeds a confirmation dialog, and a list
	// that reshuffles between renders is one a person stops reading.
	sort.Slice(est.Components, func(i, j int) bool { return est.Components[i].Name < est.Components[j].Name })
	sort.Strings(est.Unpriced)

	// Modelled, not Complete, is what distinguishes "nothing to create"
	// from "nothing I can see". A scenario that genuinely declares no
	// resources is a real, harmless case; one whose resources this
	// struct cannot carry is not.
	est.Modelled = len(est.Components) > 0 || !hasUnmodelledResources(s.Cloud)
	est.Complete = est.Modelled && len(est.Unpriced) == 0 && len(est.Components) > 0
	return est
}
