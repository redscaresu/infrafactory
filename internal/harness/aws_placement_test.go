package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	awsRunAccount   = "111122223333"
	awsOtherAccount = "444455556666"
)

func awsStateResource(mode, resourceType string, attrs map[string]any) map[string]any {
	return map[string]any{
		"mode": mode, "type": resourceType, "name": "main",
		"provider":  `provider["registry.opentofu.org/hashicorp/aws"]`,
		"instances": []any{map[string]any{"attributes": attrs}},
	}
}

func awsStateWorkDir(t *testing.T, resources ...map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	payload, err := json.Marshal(map[string]any{"resources": resources})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, LiveStateFilename), payload, 0o600))
	return dir
}

func TestUnplacedAWSResources(t *testing.T) {
	t.Parallel()
	routeTable := awsStateResource("managed", "aws_route_table", map[string]any{"id": "rtb-1", "owner_id": awsRunAccount})
	for name, tc := range map[string]struct {
		resources []map[string]any
		want      []string
	}{
		"arn account X is placed": {
			resources: []map[string]any{awsStateResource("managed", "aws_instance", map[string]any{"id": "i-1", "arn": "arn:aws:ec2:eu-west-1:" + awsRunAccount + ":instance/i-1"})},
		},
		"another account is refused, naming type, id and account": {
			resources: []map[string]any{awsStateResource("managed", "aws_instance", map[string]any{"id": "i-1", "arn": "arn:aws:ec2:eu-west-1:" + awsOtherAccount + ":instance/i-1"})},
			want:      []string{`aws_instance (i-1) in account "` + awsOtherAccount + `"`},
		},
		// fakeaws's shape: the provider skipped the account lookup and
		// built the arn client-side.
		"an empty arn account with no owner_id is refused": {
			resources: []map[string]any{awsStateResource("managed", "aws_instance", map[string]any{"id": "i-1", "arn": "arn:aws:ec2:eu-west-1::instance/i-1", "owner_id": ""})},
			want:      []string{"aws_instance (i-1) carries no account id"},
		},
		"a malformed arn is refused": {
			resources: []map[string]any{awsStateResource("managed", "aws_vpc", map[string]any{"id": "vpc-1", "arn": "vpc-1", "owner_id": awsRunAccount})},
			want:      []string{`aws_vpc (vpc-1) whose arn "vpc-1" is not an ARN`},
		},
		"owner_id X alone is placed": {
			resources: []map[string]any{awsStateResource("managed", "aws_vpc", map[string]any{"id": "vpc-1", "owner_id": awsRunAccount})},
		},
		"arn X with owner_id Y is refused": {
			resources: []map[string]any{awsStateResource("managed", "aws_vpc", map[string]any{"id": "vpc-1", "arn": "arn:aws:ec2:eu-west-1:" + awsRunAccount + ":vpc/vpc-1", "owner_id": awsOtherAccount})},
			want:      []string{`aws_vpc (vpc-1) in account "` + awsOtherAccount + `"`},
		},
		"aws_route is placed with its route table in the state": {
			resources: []map[string]any{routeTable, awsStateResource("managed", "aws_route", map[string]any{"id": "r-rtb-1", "route_table_id": "rtb-1"})},
		},
		"aws_route is refused without its route table": {
			resources: []map[string]any{awsStateResource("managed", "aws_route", map[string]any{"id": "r-rtb-1", "route_table_id": "rtb-1"})},
			want:      []string{`aws_route (r-rtb-1) whose route_table_id "rtb-1" is not in the state`},
		},
		"aws_route is refused when its route table was looked up": {
			resources: []map[string]any{
				awsStateResource("data", "aws_route_table", map[string]any{"id": "rtb-1", "owner_id": awsRunAccount}),
				awsStateResource("managed", "aws_route", map[string]any{"id": "r-rtb-1", "route_table_id": "rtb-1"}),
			},
			want: []string{`aws_route (r-rtb-1) whose route_table_id "rtb-1" is not in the state`},
		},
		"aws_route_table_association without subnet_id is refused": {
			resources: []map[string]any{routeTable, awsStateResource("managed", "aws_route_table_association", map[string]any{"id": "rtbassoc-1", "route_table_id": "rtb-1"})},
			want:      []string{`aws_route_table_association (rtbassoc-1) whose subnet_id "" is not in the state`},
		},
		"a type with neither field and off the child list is refused": {
			resources: []map[string]any{awsStateResource("managed", "aws_security_group_rule", map[string]any{"id": "sgrule-1"})},
			want:      []string{"aws_security_group_rule (sgrule-1) carries no account id"},
		},
		"a data source is ignored": {
			resources: []map[string]any{awsStateResource("data", "aws_ami", map[string]any{"id": "ami-1", "owner_id": awsOtherAccount})},
		},
		"a builtin is ignored": {
			resources: []map[string]any{{
				"mode": "managed", "type": "terraform_data", "name": "marker",
				"provider":  `provider["terraform.io/builtin/terraform"]`,
				"instances": []any{map[string]any{"attributes": map[string]any{"id": "x"}}},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := UnplacedAWSResources(awsStateWorkDir(t, tc.resources...), awsRunAccount)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// An empty list means "all placed", so a state that cannot be judged
// must never produce one.
func TestUnplacedAWSResourcesErrors(t *testing.T) {
	t.Parallel()

	got, err := UnplacedAWSResources(t.TempDir(), awsRunAccount)
	require.Error(t, err, "an unreadable state is an error")
	assert.Nil(t, got)

	got, err = UnplacedAWSResources(awsStateWorkDir(t), "")
	require.Error(t, err, "no account to place in")
	assert.Nil(t, got)
}

// Placement by account is not wired into the state-only destroy: it still
// places by project_id, so an AWS state is refused before tofu runs.
// Admitting AWS there is a deliberate later change.
func TestRunWithoutConfigRefusesAnAWSState(t *testing.T) {
	t.Parallel()
	workDir := awsStateWorkDir(t, awsStateResource("managed", "aws_vpc", map[string]any{"id": "vpc-1", "owner_id": awsRunAccount}))
	unplaced, err := UnplacedAWSResources(workDir, awsRunAccount)
	require.NoError(t, err)
	require.Empty(t, unplaced, "a state the account check places")
	runner := &fakeRunner{}

	_, err = NewSandboxDestroyHarness(runner).RunWithoutConfig(context.Background(), workDir, awsRunAccount, nil)

	require.ErrorIs(t, err, ErrProtectedProject)
	assert.Empty(t, runner.calls, "refused before tofu runs")
}
