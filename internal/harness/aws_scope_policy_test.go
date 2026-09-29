package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The policies the user applies by hand at AWS scope setup
// (docs/operations.md § Layer 3 (AWS)). REGION and ACCOUNT_ID are
// placeholders the runbook fills in: the repo is public.
const (
	awsScopeIAMPolicyFile = "../../docs/layer3/aws/iam-policy.json"
	awsScopeSCPFile       = "../../docs/layer3/aws/scp.json"
	awsScopeExemption     = "ArnNotLike aws:PrincipalArn arn:aws:iam::*:role/OrganizationAccountAccessRole"
	awsScopeRegionDeny    = "StringNotEquals aws:RequestedRegion REGION"
)

var (
	awsScopeClaimARN = "arn:aws:ssm:REGION:ACCOUNT_ID:parameter" + AWSClaimParameter
	awsScopeStampARN = "arn:aws:ssm:REGION:ACCOUNT_ID:parameter" + AWSStampParameter
)

// awsPolicyList is a policy element written as one string or a list.
type awsPolicyList []string

func (l *awsPolicyList) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*l = awsPolicyList{one}
		return nil
	}
	return json.Unmarshal(data, (*[]string)(l))
}

type awsPolicyStatement struct {
	Sid, Effect                              string
	Action, NotAction, Resource, NotResource awsPolicyList
	Condition                                map[string]map[string]awsPolicyList
}

type awsPolicy struct {
	Version   string
	Statement []awsPolicyStatement
}

// readAWSPolicy refuses an element the checks do not read, and a
// 12-digit number: a real account id in a public repo.
func readAWSPolicy(t *testing.T, file string) awsPolicy {
	t.Helper()
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	require.NotRegexp(t, `[0-9]{12}`, string(data), "%s holds an account id; write ACCOUNT_ID", file)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var policy awsPolicy
	require.NoError(t, decoder.Decode(&policy), file)
	return policy
}

// awsActionMatches is IAM's action match: case-insensitive, * and ?.
func awsActionMatches(pattern, action string) bool {
	ok, _ := path.Match(strings.ToLower(pattern), strings.ToLower(action))
	return ok
}

// awsScopeGrants is every action the key sends and the resources it
// sends it to: the claim, the sweep, and each swept collection's reap.
func awsScopeGrants() map[string][]string {
	grants := map[string][]string{
		"sts:GetCallerIdentity":  {"*"},
		"ssm:GetParameter":       {awsScopeClaimARN, awsScopeStampARN},
		"ssm:PutParameter":       {awsScopeClaimARN},
		"ssm:DeleteParameter":    {awsScopeClaimARN},
		"ssm:DescribeParameters": {"*"},
		"ec2:Describe*":          {"*"},
	}
	for _, step := range AWSReapSteps {
		if !slices.ContainsFunc(AWSSweepCollections, func(c AWSSweepCollection) bool { return c.Name == step.Collection }) {
			continue
		}
		for _, action := range step.EC2Actions {
			grants["ec2:"+action] = []string{"*"}
		}
	}
	return grants
}

// awsIAMPolicyViolations is every way policy differs from the grants:
// an action or resource beyond them, a grant nothing allows, or a
// statement on every resource not pinned to REGION.
func awsIAMPolicyViolations(policy awsPolicy) []string {
	grants := awsScopeGrants()
	var out []string
	for _, s := range policy.Statement {
		if s.Effect != "Allow" || len(s.NotAction) > 0 || len(s.NotResource) > 0 {
			out = append(out, s.Sid+": every statement allows named actions on named resources")
		}
		for _, action := range s.Action {
			reach, granted := grants[action]
			if !granted {
				out = append(out, fmt.Sprintf("%s allows %s: not the claim's, the sweep's or a swept collection's reap action, and ec2:Describe* is the one wildcard", s.Sid, action))
				continue
			}
			for _, resource := range s.Resource {
				if !slices.Contains(reach, resource) {
					out = append(out, fmt.Sprintf("%s allows %s on %s, beyond %v", s.Sid, action, resource, reach))
				}
			}
		}
		if slices.Contains(s.Resource, "*") && !slices.Equal(s.Action, awsPolicyList{"sts:GetCallerIdentity"}) &&
			!slices.Equal(s.Condition["StringEquals"]["aws:RequestedRegion"], awsPolicyList{"REGION"}) {
			out = append(out, s.Sid+" allows every resource without StringEquals aws:RequestedRegion REGION")
		}
	}
	for action, reach := range grants {
		for _, resource := range reach {
			if !slices.ContainsFunc(policy.Statement, func(s awsPolicyStatement) bool {
				return s.Effect == "Allow" && slices.Contains(s.Resource, resource) &&
					slices.ContainsFunc(s.Action, func(p string) bool { return awsActionMatches(p, action) })
			}) {
				out = append(out, fmt.Sprintf("nothing allows %s on %s", action, resource))
			}
		}
	}
	slices.Sort(out)
	return out
}

// awsSCPViolations is every way scp differs from the scope's boundary:
// one statement denies every service but ec2, ssm and sts, one denies
// every action outside REGION, each exempts only the admin role, and no
// other statement denies what the claim, sweep or reap sends.
func awsSCPViolations(scp awsPolicy) []string {
	var out []string
	var serviceDenies, regionDenies int
	for _, s := range scp.Statement {
		if s.Effect != "Deny" || !slices.Equal(s.Resource, awsPolicyList{"*"}) {
			out = append(out, s.Sid+": every statement denies on every resource")
		}
		var conditions []string
		for op, keys := range s.Condition {
			for key, values := range keys {
				conditions = append(conditions, op+" "+key+" "+strings.Join(values, ","))
			}
		}
		i := slices.Index(conditions, awsScopeRegionDeny)
		regionDeny := i >= 0 && slices.Equal(s.Action, awsPolicyList{"*"})
		if regionDeny {
			regionDenies++
			conditions = slices.Delete(conditions, i, i+1)
		}
		if !slices.Equal(conditions, []string{awsScopeExemption}) {
			out = append(out, fmt.Sprintf("%s is conditioned on %q; its one exemption is %q", s.Sid, conditions, awsScopeExemption))
		}
		if len(s.NotAction) == 0 {
			if !regionDeny {
				out = append(out, s.Sid+" is neither the service boundary nor the region boundary")
			}
			continue
		}
		serviceDenies++
		var services []string
		for _, action := range s.NotAction {
			service, _ := strings.CutSuffix(action, ":*")
			services = append(services, service)
		}
		slices.Sort(services)
		if !slices.Equal(services, []string{"ec2", "ssm", "sts"}) {
			out = append(out, fmt.Sprintf("%s spares %v; it spares exactly ec2:*, ssm:* and sts:*", s.Sid, s.NotAction))
		}
	}
	if serviceDenies != 1 {
		out = append(out, fmt.Sprintf("%d statements deny by NotAction; one denies every service but ec2, ssm and sts", serviceDenies))
	}
	if regionDenies != 1 {
		out = append(out, fmt.Sprintf("%d statements deny every action on %s; one does", regionDenies, awsScopeRegionDeny))
	}
	return out
}

func awsStatement(t *testing.T, policy *awsPolicy, sid string) *awsPolicyStatement {
	t.Helper()
	i := slices.IndexFunc(policy.Statement, func(s awsPolicyStatement) bool { return s.Sid == sid })
	require.GreaterOrEqual(t, i, 0, "no statement %s", sid)
	return &policy.Statement[i]
}

func TestAWSScopeIAMPolicyGrantsOnlyClaimSweepAndReap(t *testing.T) {
	t.Parallel()
	assert.Empty(t, awsIAMPolicyViolations(readAWSPolicy(t, awsScopeIAMPolicyFile)))
}

func TestAWSScopeIAMPolicyViolationsCatchEachWidening(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*testing.T, *awsPolicy){
		"a widened action": func(t *testing.T, p *awsPolicy) {
			reap := awsStatement(t, p, "Reap")
			reap.Action = append(reap.Action, "ec2:RunInstances")
		},
		"an ec2 wildcard": func(t *testing.T, p *awsPolicy) {
			reap := awsStatement(t, p, "Reap")
			reap.Action = append(reap.Action, "ec2:Delete*")
		},
		"an ssm wildcard": func(t *testing.T, p *awsPolicy) {
			sweep := awsStatement(t, p, "Sweep")
			sweep.Action = append(sweep.Action, "ssm:*")
		},
		"PutParameter reaching the stamp": func(t *testing.T, p *awsPolicy) {
			read := awsStatement(t, p, "ReadTheClaimAndTheStamp")
			read.Action = append(read.Action, "ssm:PutParameter")
		},
		"DeleteParameter reaching the prefix": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "WriteOnlyTheClaim").Resource = awsPolicyList{"arn:aws:ssm:REGION:ACCOUNT_ID:parameter" + AWSScopePrefix + "*"}
		},
		"a missing reap action": func(t *testing.T, p *awsPolicy) {
			reap := awsStatement(t, p, "Reap")
			reap.Action = slices.DeleteFunc(reap.Action, func(a string) bool { return a == "ec2:DeleteVpc" })
		},
		"the reap unpinned from the region": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "Reap").Condition = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			policy := readAWSPolicy(t, awsScopeIAMPolicyFile)
			mutate(t, &policy)
			assert.NotEmpty(t, awsIAMPolicyViolations(policy))
		})
	}
}

func TestAWSScopeSCPDeniesAllButEC2SSMSTSInOneRegion(t *testing.T) {
	t.Parallel()
	assert.Empty(t, awsSCPViolations(readAWSPolicy(t, awsScopeSCPFile)))
}

func TestAWSScopeSCPViolationsCatchEachWidening(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*testing.T, *awsPolicy){
		"a fourth service": func(t *testing.T, p *awsPolicy) {
			services := awsStatement(t, p, "DenyEveryServiceButEC2SSMSTS")
			services.NotAction = append(services.NotAction, "s3:*")
		},
		"a second exempt role": func(t *testing.T, p *awsPolicy) {
			exempt := awsStatement(t, p, "DenyEveryOtherRegion").Condition["ArnNotLike"]
			exempt["aws:PrincipalArn"] = append(exempt["aws:PrincipalArn"], "arn:aws:iam::*:role/Other")
		},
		"an exemption by another key": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "DenyEveryServiceButEC2SSMSTS").Condition["StringNotEquals"] = map[string]awsPolicyList{"aws:PrincipalTag/team": {"infra"}}
		},
		"an extra deny": func(t *testing.T, p *awsPolicy) {
			p.Statement = append(p.Statement, awsPolicyStatement{Sid: "Extra", Effect: "Deny", Action: awsPolicyList{"ssm:*"}, Resource: awsPolicyList{"*"},
				Condition: map[string]map[string]awsPolicyList{"ArnNotLike": {"aws:PrincipalArn": {"arn:aws:iam::*:role/OrganizationAccountAccessRole"}}}})
		},
		"the region deny dropped": func(t *testing.T, p *awsPolicy) {
			p.Statement = slices.DeleteFunc(p.Statement, func(s awsPolicyStatement) bool { return s.Sid == "DenyEveryOtherRegion" })
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scp := readAWSPolicy(t, awsScopeSCPFile)
			mutate(t, &scp)
			assert.NotEmpty(t, awsSCPViolations(scp))
		})
	}
}
