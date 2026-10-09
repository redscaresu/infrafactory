package cli

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The AWS scope is built by hand from docs/operations.md § Layer 3 (AWS),
// which the stamp's and default VPC's errors point at, so the runbook
// must name what the code reads.
func TestAWSScopeSetupRunbookNamesWhatTheCodeReads(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../docs/operations.md")
	require.NoError(t, err)
	_, section, found := strings.Cut(string(data), "\n## Layer 3 (AWS)\n")
	require.True(t, found, "docs/operations.md has no ## Layer 3 (AWS)")
	section, _, _ = strings.Cut(section, "\n## ")

	for _, want := range []string{
		"### Scope setup",
		"AWSClaimParameter", harness.AWSClaimParameter,
		"AWSStampParameter", harness.AWSStampParameter,
		"~/" + awsCredentialFile, "0600",
		"docs/layer3/aws/iam-policy.json", "docs/layer3/aws/scp.json",
		"arn:aws:iam::*:role/OrganizationAccountAccessRole",
	} {
		assert.Contains(t, section, want)
	}
}

// The run checklist is what an operator reads before spending money, so
// it must name the stages and the reap command a failure prints, and
// list no IAM actions: the policy file is the one list.
func TestAWSRunChecklistNamesTheStagesAndTheReapCommand(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../docs/operations.md")
	require.NoError(t, err)
	_, section, found := strings.Cut(string(data), "\n### Running aws-web-live\n")
	require.True(t, found, "docs/operations.md has no ### Running aws-web-live")
	// Cut at the next heading, not at "\n#", which a shell comment in a
	// fenced block would also match.
	if i := regexp.MustCompile(`\n#{2,3} `).FindStringIndex(section); i != nil {
		section = section[:i[0]]
	}
	_, inLayer3AWS, _ := strings.Cut(string(data), "\n## Layer 3 (AWS)\n")
	inLayer3AWS, _, _ = strings.Cut(inLayer3AWS, "\n## ")
	require.Contains(t, inLayer3AWS, "\n### Running aws-web-live\n", "the checklist must sit under ## Layer 3 (AWS)")

	for _, want := range []string{
		StageAWSAMIResolve,
		StageAWSScopeClaimKept,
		reapCommand(config.DefaultPath, "scenarios/training/aws-web-live.yaml"),
		"docs/layer3/aws/iam-policy.json",
	} {
		assert.Contains(t, section, want)
	}
	// Any service:Action, so a new action of any service fails, not only
	// of a listed few.
	assert.Equal(t, []string{"ssm:GetParameter"}, regexp.MustCompile(`\b[a-z0-9-]+:[A-Z][A-Za-z*]+\b`).FindAllString(section, -1),
		"the checklist names one action, in the sentence about the public parameter")
}
