package harness

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The policy the user attaches to the read-only reader user
// (docs/operations.md § The reader policy), with the same REGION and
// ACCOUNT_ID placeholders as the Layer 3 policy.
const awsReaderPolicyFile = "../../docs/layer3/aws/reader-policy.json"

// awsReaderPublicParameters is the one resource outside ACCOUNT_ID the
// reader may name: the public SSM parameters, which have no account.
const awsReaderPublicParameters = "arn:aws:ssm:REGION::parameter/aws/service/*"

// awsARNInTheAccount reports whether arn names ACCOUNT_ID, and REGION or
// no region (a global service such as IAM).
func awsARNInTheAccount(arn string) bool {
	f := strings.SplitN(arn, ":", 6)
	return len(f) == 6 && f[0] == "arn" && f[1] == "aws" && (f[3] == "" || f[3] == "REGION") && f[4] == "ACCOUNT_ID"
}

// awsReaderTableActions is every reader-table row as a policy action.
func awsReaderTableActions() []string {
	var out []string
	for _, svc := range AWSReaderTable {
		for _, action := range svc.Actions {
			out = append(out, svc.Service+":"+action)
		}
	}
	return out
}

// awsReaderPolicyViolations is every way policy differs from rows: a
// statement other than Allow on named actions, an action that is not a
// row (a wildcard never is), a row nothing allows, a DryRun-table
// action, which the preflight proves the key cannot send, a statement on
// every resource holding anything but ec2 actions or not pinned to REGION
// (sts:GetCallerIdentity alone excepted), or any other resource outside
// ACCOUNT_ID but the public parameters.
func awsReaderPolicyViolations(policy awsPolicy, rows []string) []string {
	var out []string
	granted := map[string]bool{}
	for _, s := range policy.Statement {
		if s.Effect != "Allow" || len(s.NotAction) > 0 || len(s.NotResource) > 0 {
			out = append(out, s.Sid+": every statement allows named actions on named resources")
		}
		for _, action := range s.Action {
			granted[action] = true
			if !slices.Contains(rows, action) {
				out = append(out, fmt.Sprintf("%s allows %s: not a reader-table row", s.Sid, action))
			}
			for _, dryRun := range AWSReaderDryRunActions {
				if awsActionMatches(action, awsDryRunService+":"+dryRun) {
					out = append(out, fmt.Sprintf("%s allows %s, which reaches DryRun-table action %s", s.Sid, action, dryRun))
				}
			}
		}
		if slices.Contains(s.Resource, "*") && !slices.Equal(s.Action, awsPolicyList{"sts:GetCallerIdentity"}) {
			if slices.ContainsFunc(s.Action, func(a string) bool { return !strings.HasPrefix(a, "ec2:") }) {
				out = append(out, fmt.Sprintf("%s allows every resource for %v; only ec2 actions take *", s.Sid, s.Action))
			}
			if !slices.Equal(s.Condition["StringEquals"]["aws:RequestedRegion"], awsPolicyList{"REGION"}) {
				out = append(out, s.Sid+" allows every resource without StringEquals aws:RequestedRegion REGION")
			}
		}
		for _, resource := range s.Resource {
			if resource != "*" && resource != awsReaderPublicParameters && !awsARNInTheAccount(resource) {
				out = append(out, fmt.Sprintf("%s allows %s: not in ACCOUNT_ID and REGION", s.Sid, resource))
			}
		}
	}
	for _, row := range rows {
		if !granted[row] {
			out = append(out, "nothing allows reader-table row "+row)
		}
	}
	slices.Sort(out)
	return out
}

func TestAWSReaderPolicyIsExactlyTheReaderTable(t *testing.T) {
	t.Parallel()
	rows := awsReaderTableActions()
	require.NotEmpty(t, rows)
	assert.Empty(t, awsReaderPolicyViolations(readAWSPolicy(t, awsReaderPolicyFile), rows))
}

func TestAWSReaderPolicyViolationsCatchEachWidening(t *testing.T) {
	t.Parallel()
	add := func(sid, action string) func(*testing.T, *awsPolicy) {
		return func(t *testing.T, p *awsPolicy) {
			s := awsStatement(t, p, sid)
			s.Action = append(s.Action, action)
		}
	}
	require.Contains(t, AWSReaderDryRunActions, "DeleteVpc")
	for name, mutate := range map[string]func(*testing.T, *awsPolicy){
		"a write":             add("ReadTheNetwork", "ec2:CreateVpc"),
		"an ec2 wildcard":     add("ReadTheNetwork", "ec2:*"),
		"an ssm put wildcard": add("ReadTheParameters", "ssm:Put*"),
		"an iam wildcard":     add("ReadTheRoles", "iam:*"),
		"a DryRun action":     add("ReadTheNetwork", "ec2:DeleteVpc"),
		"a NotAction statement": func(_ *testing.T, p *awsPolicy) {
			p.Statement = append(p.Statement, awsPolicyStatement{Sid: "Everything", Effect: "Allow",
				NotAction: awsPolicyList{"iam:*"}, Resource: awsPolicyList{"*"},
				Condition: map[string]map[string]awsPolicyList{"StringEquals": {"aws:RequestedRegion": {"REGION"}}}})
		},
		"a Deny statement": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "Identity").Effect = "Deny"
		},
		"a NotResource statement": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "Identity").NotResource = awsPolicyList{"arn:aws:iam::ACCOUNT_ID:root"}
		},
		"the region pin removed": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "ReadTheNetwork").Condition = nil
		},
		"the roles on every resource": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "ReadTheRoles").Resource = awsPolicyList{"*"}
		},
		"the parameters on every resource": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "ReadTheParameters").Resource = awsPolicyList{"*"}
		},
		"the roles on every resource, pinned to REGION": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "ReadTheRoles").Resource = awsPolicyList{"*"}
			awsStatement(t, p, "ReadTheRoles").Condition = awsStatement(t, p, "ReadTheNetwork").Condition
		},
		"the parameters on every resource, pinned to REGION": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "ReadTheParameters").Resource = awsPolicyList{"*"}
			awsStatement(t, p, "ReadTheParameters").Condition = awsStatement(t, p, "ReadTheNetwork").Condition
		},
		"a role in another account": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "ReadTheRoles").Resource = awsPolicyList{"arn:aws:iam::123456789012:role/*"}
		},
		"the parameters in every region": func(t *testing.T, p *awsPolicy) {
			awsStatement(t, p, "ReadTheParameters").Resource = awsPolicyList{"arn:aws:ssm:*:ACCOUNT_ID:parameter/*"}
		},
		"a dropped row": func(t *testing.T, p *awsPolicy) {
			network := awsStatement(t, p, "ReadTheNetwork")
			network.Action = slices.DeleteFunc(network.Action, func(a string) bool { return a == "ec2:DescribeVpcs" })
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			policy := readAWSPolicy(t, awsReaderPolicyFile)
			mutate(t, &policy)
			assert.NotEmpty(t, awsReaderPolicyViolations(policy, awsReaderTableActions()))
		})
	}
}

// A DryRun-table action added to the reader table and the policy alike
// still fails: the preflight proves the key cannot send it.
func TestAWSReaderPolicyViolationsRefuseADryRunActionAsARow(t *testing.T) {
	t.Parallel()
	policy := readAWSPolicy(t, awsReaderPolicyFile)
	network := awsStatement(t, &policy, "ReadTheNetwork")
	network.Action = append(network.Action, "ec2:DeleteVpc")
	rows := append(awsReaderTableActions(), "ec2:DeleteVpc")
	assert.Equal(t, []string{"ReadTheNetwork allows ec2:DeleteVpc, which reaches DryRun-table action DeleteVpc"},
		awsReaderPolicyViolations(policy, rows))
}

func TestADR0023AmendmentNamesTheReaderPolicy(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(adr0023File)
	require.NoError(t, err)
	for _, want := range []string{"reader-policy.json", "read-only", "key file", "delete-access-key"} {
		assert.Contains(t, string(data), want)
	}
}
