package scenario

import (
	"errors"
	"fmt"
)

var ErrInvalidCriterionSpec = errors.New("invalid acceptance criterion spec")

type CriterionSpecError struct {
	Index int
	Type  string
	Field string
	Err   error
}

func (e *CriterionSpecError) Error() string {
	if e == nil {
		return ErrInvalidCriterionSpec.Error()
	}
	return fmt.Sprintf("%s: index=%d type=%q field=%q: %v", ErrInvalidCriterionSpec, e.Index, e.Type, e.Field, e.Err)
}

func (e *CriterionSpecError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *CriterionSpecError) Is(target error) bool {
	return target == ErrInvalidCriterionSpec
}

type ExecutableCheckSpec struct {
	Type        string
	Expect      string
	Description string

	Connectivity  *ConnectivityCheckSpec
	HTTPProbe     *HTTPProbeCheckSpec
	Policy        *PolicyCheckSpec
	Destruction   *DestructionCheckSpec
	DNSResolution *DNSResolutionCheckSpec
}

type ConnectivityCheckSpec struct {
	From string
	To   string
	Port int
}

type HTTPProbeCheckSpec struct {
	Target string
	Port   int
}

type PolicyCheckSpec struct {
	Check  string
	Target string
}

type DestructionCheckSpec struct{}

type DNSResolutionCheckSpec struct {
	Domain string
}

func (s Scenario) ExecutableChecks() ([]ExecutableCheckSpec, error) {
	return ParseAcceptanceCriteria(s.AcceptanceCriteria)
}

func ParseAcceptanceCriteria(criteria []AcceptanceCriterion) ([]ExecutableCheckSpec, error) {
	specs := make([]ExecutableCheckSpec, 0, len(criteria))

	for i, criterion := range criteria {
		spec := ExecutableCheckSpec{
			Type:        criterion.Type,
			Expect:      criterion.Expect,
			Description: criterion.Description,
		}

		if spec.Expect == "" {
			return nil, &CriterionSpecError{
				Index: i,
				Type:  criterion.Type,
				Field: "expect",
				Err:   errors.New("is required"),
			}
		}

		switch criterion.Type {
		case "connectivity":
			if criterion.From == "" {
				return nil, &CriterionSpecError{
					Index: i,
					Type:  criterion.Type,
					Field: "from",
					Err:   errors.New("is required"),
				}
			}
			if criterion.To == "" {
				return nil, &CriterionSpecError{
					Index: i,
					Type:  criterion.Type,
					Field: "to",
					Err:   errors.New("is required"),
				}
			}

			port := 0
			if criterion.Port != nil {
				port = *criterion.Port
			}
			spec.Connectivity = &ConnectivityCheckSpec{
				From: criterion.From,
				To:   criterion.To,
				Port: port,
			}
		case "http_probe":
			if criterion.Target == "" {
				return nil, &CriterionSpecError{
					Index: i,
					Type:  criterion.Type,
					Field: "target",
					Err:   errors.New("is required"),
				}
			}
			if criterion.Port == nil {
				return nil, &CriterionSpecError{
					Index: i,
					Type:  criterion.Type,
					Field: "port",
					Err:   errors.New("is required"),
				}
			}

			spec.HTTPProbe = &HTTPProbeCheckSpec{
				Target: criterion.Target,
				Port:   *criterion.Port,
			}
		case "policy":
			if criterion.Check == "" {
				return nil, &CriterionSpecError{
					Index: i,
					Type:  criterion.Type,
					Field: "check",
					Err:   errors.New("is required"),
				}
			}
			spec.Policy = &PolicyCheckSpec{
				Check:  criterion.Check,
				Target: criterion.Target,
			}
		case "destruction":
			spec.Destruction = &DestructionCheckSpec{}
		case "dns_resolution":
			if criterion.Domain == "" {
				return nil, &CriterionSpecError{
					Index: i,
					Type:  criterion.Type,
					Field: "domain",
					Err:   errors.New("is required"),
				}
			}
			spec.DNSResolution = &DNSResolutionCheckSpec{
				Domain: criterion.Domain,
			}
		default:
			return nil, &CriterionSpecError{
				Index: i,
				Type:  criterion.Type,
				Field: "type",
				Err:   fmt.Errorf("unsupported criterion type %q", criterion.Type),
			}
		}

		specs = append(specs, spec)
	}

	return specs, nil
}

