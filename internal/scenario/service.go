package scenario

import (
	"fmt"
	"strings"
	"time"
)

// MaxServiceTTL bounds how long any single deployment may live. It is not
// a cost control -- at roughly EUR 0.042/hour for the lb-serving-paris
// shape, a week is small money (ADR-0024). It is a typo control: `400h`
// where `4h` was meant should be refused at validation rather than
// discovered on an invoice.
const MaxServiceTTL = 168 * time.Hour

// DefaultHealthPath is polled to decide whether the service is serving.
const DefaultHealthPath = "/"

// mutableTags are tags that conventionally move. Pinning to one defeats
// the only property a live service needs from its image -- being able to
// name the version that is running -- so an upgrade from any of these
// could not be told from a no-op, and a soak failure could not be
// attributed to a version.
//
// This catches the common case; it is not a proof of immutability. A
// numeric tag like `1` moves too, and only a digest (`@sha256:...`) is
// genuinely fixed. Digest pinning is worth doing and is not done here.
var mutableTags = map[string]bool{
	"latest":   true,
	"stable":   true,
	"edge":     true,
	"main":     true,
	"master":   true,
	"nightly":  true,
	"rolling":  true,
	"devel":    true,
	"unstable": true,
}

// ServiceSpec is the versioned application a live-service scenario runs on
// the infrastructure it declares.
type ServiceSpec struct {
	Image      string `json:"image"`
	Tag        string `json:"tag"`
	Port       int    `json:"port"`
	HealthPath string `json:"health_path,omitempty"`
	TTL        string `json:"ttl"`
}

// Ref is the fully qualified image reference the instance pulls.
func (s ServiceSpec) Ref() string {
	return s.Image + ":" + s.Tag
}

// TimeToLive parses the declared TTL.
func (s ServiceSpec) TimeToLive() (time.Duration, error) {
	ttl, err := time.ParseDuration(s.TTL)
	if err != nil {
		return 0, fmt.Errorf("parse ttl %q: %w", s.TTL, err)
	}
	return ttl, nil
}

// Validate enforces the rules the JSON schema cannot express. The schema
// already guarantees the fields are present and well-shaped; this covers
// what "well-shaped" does not: a tag that names nothing stable, and a TTL
// that is zero, negative, or long enough to be a typo.
func (s ServiceSpec) Validate() error {
	if mutableTags[strings.ToLower(strings.TrimSpace(s.Tag))] {
		return fmt.Errorf(
			"service.tag %q is a moving tag: pin an immutable version (e.g. \"1.27\"), "+
				"because an upgrade from a tag that moves cannot be told from a no-op",
			s.Tag)
	}

	ttl, err := s.TimeToLive()
	if err != nil {
		return fmt.Errorf("service.ttl: %w", err)
	}
	if ttl <= 0 {
		return fmt.Errorf("service.ttl %q must be positive: live deployments have no unbounded form", s.TTL)
	}
	if ttl > MaxServiceTTL {
		return fmt.Errorf("service.ttl %q exceeds the %s maximum", s.TTL, MaxServiceTTL)
	}

	return nil
}

// validateService applies the service rules to a loaded scenario. A
// scenario without a service block is not a live-service scenario and is
// unaffected.
func validateService(sc *Scenario) error {
	if sc.Service == nil {
		return nil
	}
	if err := sc.Service.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidScenario, err)
	}
	return nil
}
