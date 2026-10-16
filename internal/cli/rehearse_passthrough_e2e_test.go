package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/e2e"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// passthroughStepTimeout bounds each passthrough command the e2e runs:
// its context is cancelled at the deadline, which interrupts tofu.
const passthroughStepTimeout = 5 * time.Minute

// passthroughPrepareTimeout bounds prepare, whose two tofu inits may each
// take rehearsalTofuTimeout, plus a margin for starting fakeaws.
const passthroughPrepareTimeout = 2*cli.RehearsalTofuTimeout + 5*time.Minute

// Tofu's own exit codes, which passthrough returns unchanged: 1 is an
// error, and 2 is plan -detailed-exitcode's "changes present".
const (
	tofuExitError   = 1
	tofuExitChanges = 2
)

// The CIDRs of the VPC the root creates, which the var file sets, and of
// the one the test creates in fakeaws outside the state and imports.
const (
	passthroughVarCIDR    = "10.0.0.0/16"
	passthroughImportCIDR = "10.1.0.0/16"
)

// passthroughRoot is the user's root: aws_vpc.main from var.cidr, which
// has no default, so only the rehearsal's var file can set it, and
// aws_vpc.imported, which the test imports rather than applies.
var passthroughRoot = fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = %q
    }
  }
}

`, harness.AWSProviderVersion) + providerBlock(fmt.Sprintf("  region = %q\n", assumptionRegion)) + `variable "cidr" {
  type = string
}

resource "aws_vpc" "main" {
  cidr_block = var.cidr
}

resource "aws_vpc" "imported" {
  cidr_block = "` + passthroughImportCIDR + `"
}
`

// TestRehearsePassthroughE2E prepares a root with a var file, then runs
// every passthrough action through the cobra commands with real tofu
// against the rehearsal's fakeaws, asserting each step's exact exit code
// and what it changed: plan -out writes p and plans aws_vpc.main; apply
// creates it in fakeaws at the var file's CIDR and in the local state;
// state mv renames it; import records a VPC made in fakeaws outside the
// state; destroy removes both from fakeaws. The refusals run before tofu
// does: console, -var, and apply with a plan file. apply without
// -auto-approve fails in tofu: -input=false does not stop tofu asking for
// approval, but tofu's stdin is empty, so it reads EOF and does not wait.
func TestRehearsePassthroughE2E(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	bin := fakeawsAtPin(t)

	repo := realTempDir(t)
	lock := fmt.Sprintf("provider %q {\n  version     = %q\n  constraints = %q\n}\n", "registry.opentofu.org/hashicorp/aws", harness.AWSProviderVersion, harness.AWSProviderVersion)
	for name, content := range map[string]string{
		".git/HEAD":                     "ref: refs/heads/main\n",
		"envs/prod/.terraform.lock.hcl": lock,
		"envs/prod/main.tf":             passthroughRoot,
		"prod.tfvars":                   fmt.Sprintf("cidr = %q\n", passthroughVarCIDR),
	} {
		e2e.WriteFile(t, filepath.Join(repo, name), []byte(content))
	}
	t.Chdir(repo)
	t.Cleanup(func() { stopRecordedFakeaws(t, repo) })

	code, stdout, stderr := rehearseStep(t, passthroughPrepareTimeout, "prepare", "envs/prod", "--account", wallsAccount, "--region", assumptionRegion, "--fakeaws-bin", bin, "--var-file", "prod.tfvars", "--output", "json")
	require.Equal(t, cli.ExitCodeSuccess, code, "%s\n%s", stdout, stderr)
	var result struct {
		ID      string `json:"id"`
		WorkDir string `json:"work_dir"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result), stdout)
	rehearsal := filepath.Join(repo, ".infrafactory", "rehearsals", result.ID)
	record := readPrepareRecord(t, rehearsal)
	state := filepath.Join(rehearsal, "state", "terraform.tfstate")
	id := result.ID

	t.Run("console is refused", func(t *testing.T) {
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "console", id)
		assert.Equal(t, cli.ExitCodeUsage, code, "%s\n%s", stdout, stderr)
		assert.Contains(t, stderr, `unknown command "console"`)
	})
	t.Run("plan -var is refused before tofu runs", func(t *testing.T) {
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "plan", id, "--", "-var=cidr=10.9.0.0/16")
		assert.Equal(t, cli.ExitCodeRuntime, code)
		assert.Contains(t, stderr, "tofu plan: -var is refused")
		assert.Empty(t, stdout, "tofu ran")
	})
	t.Run("plan -out -detailed-exitcode", func(t *testing.T) {
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "plan", id, "--", "-out=p", "-detailed-exitcode", "-no-color", "-target=aws_vpc.main")
		require.Equal(t, tofuExitChanges, code, "%s\n%s", stdout, stderr)
		assert.Contains(t, stdout, "aws_vpc.main will be created")
		assert.Regexp(t, `cidr_block\s+= "`+regexp.QuoteMeta(passthroughVarCIDR)+`"`, stdout)
		assert.Contains(t, stdout, "Plan: 1 to add, 0 to change, 0 to destroy.")
		assert.FileExists(t, filepath.Join(result.WorkDir, "p"))
	})
	t.Run("apply with a plan file is refused", func(t *testing.T) {
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "apply", id, "--", "p")
		assert.Equal(t, cli.ExitCodeRuntime, code)
		assert.Contains(t, stderr, `tofu apply: the plan file "p" is refused`)
		assert.Empty(t, stdout, "tofu ran")
	})
	t.Run("apply without -auto-approve fails in tofu", func(t *testing.T) {
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "apply", id, "--", "-no-color", "-target=aws_vpc.main")
		assert.Equal(t, tofuExitError, code, "%s\n%s", stdout, stderr)
		assert.Contains(t, stderr, "Error: error asking for approval: EOF")
		assert.Empty(t, passthroughVPCs(t, record.Port, record.Token), "tofu applied without approval")
	})

	var mainID string
	t.Run("apply -auto-approve", func(t *testing.T) {
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "apply", id, "--", "-auto-approve", "-no-color", "-target=aws_vpc.main")
		require.Equal(t, cli.ExitCodeSuccess, code, "%s\n%s", stdout, stderr)
		vpcs := passthroughVPCs(t, record.Port, record.Token)
		require.Len(t, vpcs, 1)
		mainID = vpcs[0].ID
		assert.Equal(t, passthroughVarCIDR, vpcs[0].CIDR)
		assert.Equal(t, map[string]string{"aws_vpc.main": mainID}, passthroughState(t, state))
	})
	require.NotEmpty(t, mainID)
	t.Run("state mv", func(t *testing.T) {
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "state", "mv", id, "--", "aws_vpc.main", "aws_vpc.moved")
		require.Equal(t, cli.ExitCodeSuccess, code, "%s\n%s", stdout, stderr)
		assert.Equal(t, map[string]string{"aws_vpc.moved": mainID}, passthroughState(t, state))
	})
	t.Run("import", func(t *testing.T) {
		outside := passthroughCreateVPC(t, record.Port, passthroughImportCIDR)
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "import", id, "--", "-no-color", "aws_vpc.imported", outside)
		require.Equal(t, cli.ExitCodeSuccess, code, "%s\n%s", stdout, stderr)
		assert.Equal(t, map[string]string{"aws_vpc.moved": mainID, "aws_vpc.imported": outside}, passthroughState(t, state))
	})
	t.Run("destroy -auto-approve", func(t *testing.T) {
		code, stdout, stderr := rehearseStep(t, passthroughStepTimeout, "destroy", id, "--", "-auto-approve", "-no-color")
		require.Equal(t, cli.ExitCodeSuccess, code, "%s\n%s", stdout, stderr)
		assert.Empty(t, passthroughVPCs(t, record.Port, record.Token))
		assert.Empty(t, passthroughState(t, state))
	})
}

// rehearseStep runs `infrafactory rehearse args...` in-process under
// timeout and reports its error through cli.ReportError, as the binary
// does, and returns the exit code and output.
func rehearseStep(t *testing.T, timeout time.Duration, args ...string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	root := cli.NewRootCmd()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(append([]string{"rehearse"}, args...))
	code := cli.ReportError(root.ExecuteContext(ctx), stderr)
	t.Logf("rehearse %s\n%s%s", strings.Join(args, " "), stdout, stderr)
	require.NoError(t, ctx.Err(), "rehearse %s ran past %s", strings.Join(args, " "), timeout)
	return code, stdout.String(), stderr.String()
}

// passthroughVPC is a VPC in fakeaws's /mock/state.
type passthroughVPC struct {
	ID   string `json:"id"`
	CIDR string `json:"cidr_block"`
}

// passthroughVPCs is every VPC fakeaws holds, read with the admin token
// through no proxy.
func passthroughVPCs(t *testing.T, port int, token string) []passthroughVPC {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/mock/state/ec2", port), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{}}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var state struct {
		EC2 *struct {
			VPCs []passthroughVPC `json:"vpcs"`
		} `json:"ec2"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	require.NotNil(t, state.EC2, "fakeaws's state has no ec2 block")
	return state.EC2.VPCs
}

// passthroughVPCID is the VpcId in a CreateVpc response.
var passthroughVPCID = regexp.MustCompile(`<vpcId>(vpc-[0-9a-f]+)</vpcId>`)

// passthroughCreateVPC makes a VPC in fakeaws with EC2's own CreateVpc,
// outside tofu, through no proxy, and returns its ID.
func passthroughCreateVPC(t *testing.T, port int, cidr string) string {
	t.Helper()
	form := url.Values{"Action": {"CreateVpc"}, "Version": {"2016-11-15"}, "CidrBlock": {cidr}}
	resp, err := (&http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{}}).PostForm(
		fmt.Sprintf("http://127.0.0.1:%d/ec2/region/%s", port, assumptionRegion), form)
	require.NoError(t, err)
	defer resp.Body.Close()
	body := new(bytes.Buffer)
	_, err = body.ReadFrom(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, body.String())
	m := passthroughVPCID.FindStringSubmatch(body.String())
	require.NotNil(t, m, body.String())
	return m[1]
}

// passthroughState maps each resource address in the local state file to
// its instance's id.
func passthroughState(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var state struct {
		Resources []struct {
			Type, Name string
			Instances  []struct {
				Attributes struct {
					ID string `json:"id"`
				} `json:"attributes"`
			} `json:"instances"`
		} `json:"resources"`
	}
	require.NoError(t, json.Unmarshal(raw, &state), string(raw))
	ids := map[string]string{}
	for _, r := range state.Resources {
		require.Len(t, r.Instances, 1, "%s.%s", r.Type, r.Name)
		ids[r.Type+"."+r.Name] = r.Instances[0].Attributes.ID
	}
	return ids
}
