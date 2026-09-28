package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plans are hashicorp/aws 5.100.0 plans captured against fakeaws by
// testdata/instance-profile/aws/capture.sh.
func TestAWSNoInstanceProfileCapturedPlans(t *testing.T) {
	t.Parallel()
	policy := filepath.Join(opaTestRepoRoot(t), "policies", "aws", "no_instance_profile.rego")
	denied := func(address string) []string {
		return []string{address + " sets iam_instance_profile — instances MUST NOT launch with an instance profile; remove the iam_instance_profile argument or block, and any aws_iam_instance_profile created for it"}
	}
	cases := []struct {
		plan string
		want []string
	}{
		{"literal_name", denied("aws_instance.web")},
		{"unknown_name", denied("aws_instance.web")},
		{"child_module", denied("module.web.aws_instance.web")},
		{"launch_template", denied("aws_launch_template.web")},
		{"launch_template_dynamic", denied("aws_launch_template.web")},
		{"launch_template_unknown_count", denied("aws_launch_template.web")},
		{"spot_instance", denied("aws_spot_instance_request.web")},
		{"launch_configuration", denied("aws_launch_configuration.web")},
		{"launch_template_no_profile", nil},
		{"web_step_one", nil},
	}
	for _, tc := range cases {
		t.Run(tc.plan, func(t *testing.T) {
			t.Parallel()
			plan, err := os.ReadFile(filepath.Join("testdata", "instance-profile", "aws", tc.plan+".json"))
			require.NoError(t, err)

			failures, err := EvaluatePlanPolicies(t.Context(), plan, []string{policy})
			require.NoError(t, err)

			var got []string
			for _, f := range failures {
				got = append(got, f.Detail)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
