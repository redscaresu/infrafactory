package e2e

import (
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

const (
	awsWebStepOneDir            = "aws-web-step-one"
	awsWebStepOneName           = "web-step-one"
	awsWebStepOneUnattachedName = "web-step-one-unattached"
	awsWebStepOneRegion         = "us-east-1"
)

// goldenAWSWebStepOneUserData is the byte-for-byte script
// renderAWSUserData (internal/cli/aws_user_data.go) produces for this
// scenario's service: block (nginx:1.27 on port 80) — mirrored here
// because the renderer and its golden constant are unexported to
// internal/cli's own test file.
const goldenAWSWebStepOneUserData = `#!/bin/bash
# Rendered by infrafactory from the scenario's service: block.
set -euo pipefail
dnf install -y docker
systemctl enable --now docker
docker run -d --restart=always -p 80:80 'nginx:1.27'
`

// TestAWSWebStepOneFixtureHasNoProviderBlockAndUserDataLine is ungated:
// no fakeaws or tofu needed. infrafactory writes the terraform{}/
// provider{} blocks and the user-data script itself (ADR-0039), so the
// checked-in fixture must contain neither, must reference the script
// with exactly generator.AWSUserDataLine, boot harness.AWSLayer2AMI (the
// id Layer 2 hands the model), and the scenario YAML must pass schema
// validation.
func TestAWSWebStepOneFixtureHasNoProviderBlockAndUserDataLine(t *testing.T) {
	fixtures := filepath.Join(RepoRoot(t), "internal", "e2e", "testdata", awsWebStepOneDir)

	for _, name := range []string{awsWebStepOneName, awsWebStepOneUnattachedName} {
		hcl, err := os.ReadFile(filepath.Join(fixtures, name+".tf"))
		require.NoError(t, err)
		content := string(hcl)
		assert.NotContains(t, content, "terraform {", "%s.tf", name)
		assert.NotContains(t, content, `provider "aws"`, "%s.tf", name)
		assert.Contains(t, content, generator.AWSUserDataLine, "%s.tf", name)
		assert.Regexp(t, `ami\s+= "`+regexp.QuoteMeta(harness.AWSLayer2AMI)+`"`, content, "%s.tf", name)

		schemaPath := filepath.Join(RepoRoot(t), "scenario.schema.json")
		_, err = scenario.LoadWithSchema(filepath.Join(fixtures, name+".yaml"), schemaPath)
		require.NoError(t, err, "%s.yaml schema validation", name)
	}
}

// startAWSWebStepOne compiles+starts fakeaws, seals the network, and
// returns a func that runs `run` on testdata/aws-web-step-one/<name>.yaml
// with <name>.tf as the generated HCL, plus the mock and the config's
// output root (for reading the generated providers.tf back).
func startAWSWebStepOne(t *testing.T, name string) (run func(args ...string) InfrafactoryResult, mock *MockwayInstance, outputRoot string) {
	t.Helper()
	SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	mock = StartFakeaws(t)
	SealNetwork(t)

	fixtures := filepath.Join(RepoRoot(t), "internal", "e2e", "testdata", awsWebStepOneDir)
	hcl, err := os.ReadFile(filepath.Join(fixtures, name+".tf"))
	require.NoError(t, err)

	workspace := t.TempDir()
	outputRoot = filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	WriteConfigMultiCloud(t, configPath, "http://127.0.0.1:1", "", mock.URL, "", outputRoot)

	run = func(args ...string) InfrafactoryResult {
		return RunInfrafactory(t, InfrafactoryRunOptions{
			Args:           append([]string{"run", filepath.Join(fixtures, name+".yaml"), "--config", configPath}, args...),
			GeneratorFiles: map[string][]byte{"main.tf": hcl},
		})
	}
	return run, mock, outputRoot
}

// TestE2E_AWSWebStepOne applies, converges and destroys the fixed
// step-one fixture on fakeaws: one VPC-networked EC2 instance running
// nginx, reachable on port 80 (http_probe compute:80, ADR-0013). It
// also proves the bytes fakeaws stored for the instance's UserData —
// what actually reached the instance, not the file on disk — equal
// the renderer's golden script byte for byte.
func TestE2E_AWSWebStepOne(t *testing.T) {
	run, mock, outputRoot := startAWSWebStepOne(t, awsWebStepOneName)

	applied := run("--no-destroy")
	require.NoError(t, applied.Err, "stdout:\n%s\nfakeaws log: %s", applied.Stdout, mock.LogPath())
	assert.Contains(t, applied.Stdout, "Status: success")
	assert.Contains(t, applied.Stdout, "run/terminal_reason: pass (target_reached)")

	providers, err := os.ReadFile(filepath.Join(outputRoot, "aws-web-step-one", "providers.tf"))
	require.NoError(t, err)
	providersContent := string(providers)
	assert.Contains(t, providersContent, `version = "5.100.0"`)
	assert.NotContains(t, providersContent, "endpoints")

	instanceID := awsSoleInstanceID(t, mock)
	userData := fetchAWSInstanceUserData(t, mock.URL, awsWebStepOneRegion, instanceID)
	assert.Equal(t, goldenAWSWebStepOneUserData, userData)

	destroyed := run()
	require.NoError(t, destroyed.Err, "stdout:\n%s\nfakeaws log: %s", destroyed.Stdout, mock.LogPath())
	assert.Contains(t, destroyed.Stdout, "run/terminal_reason: pass (target_reached)")
	assert.Zero(t, awsStateItemCount(mock.FetchState(t), "ec2", "instances"))
}

// TestE2E_AWSWebStepOneUnattachedGroup runs the same fixture with the
// port-80 ingress rule moved to a security group the instance never
// attaches. http_probe compute:80 must fail closed with the
// unattached-group diagnostic, not read "reachable" from a group that
// exists but has no effect on the instance.
func TestE2E_AWSWebStepOneUnattachedGroup(t *testing.T) {
	run, _, _ := startAWSWebStepOne(t, awsWebStepOneUnattachedName)

	result := run("--repair-iterations-max", "1")
	require.Error(t, result.Err, "stdout:\n%s", result.Stdout)
	output := result.Stdout + result.Stderr
	assert.Contains(t, output, "check=http_probe")
	assert.Contains(t, output, "no security group attached to instance")
}

// awsSoleInstanceID reads the one EC2 instance id out of fakeaws's
// /mock/state (gatherEC2StateReal's ec2.instances shape).
func awsSoleInstanceID(t *testing.T, mock *MockwayInstance) string {
	t.Helper()
	state := mock.FetchState(t)
	ec2, _ := state["ec2"].(map[string]any)
	instances, _ := ec2["instances"].([]any)
	require.Len(t, instances, 1, "expected exactly one ec2 instance in state")
	instance, _ := instances[0].(map[string]any)
	id, _ := instance["id"].(string)
	require.NotEmpty(t, id, "instance has no id field")
	return id
}

// fetchAWSInstanceUserData calls fakeaws's EC2 Query-RPC
// DescribeInstanceAttribute(userData) directly (POST
// /ec2/region/<region>, form-encoded, XML response — no SigV4 needed
// against fakeaws) and base64-decodes the returned value: the AWS
// provider base64-encodes user_data on the wire, and fakeaws stores and
// returns it verbatim.
func fetchAWSInstanceUserData(t *testing.T, fakeawsURL, region, instanceID string) string {
	t.Helper()
	params := neturl.Values{}
	params.Set("Action", "DescribeInstanceAttribute")
	params.Set("Version", "2016-11-15")
	params.Set("InstanceId", instanceID)
	params.Set("Attribute", "userData")

	req, err := http.NewRequest(http.MethodPost, fakeawsURL+"/ec2/region/"+region, strings.NewReader(params.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "DescribeInstanceAttribute body:\n%s", body)

	var decoded struct {
		UserData struct {
			Value string `xml:"value"`
		} `xml:"userData"`
	}
	require.NoError(t, xml.Unmarshal(body, &decoded), "unmarshal DescribeInstanceAttribute response:\n%s", body)

	raw, err := base64.StdEncoding.DecodeString(decoded.UserData.Value)
	require.NoError(t, err, "base64-decode userData value %q", decoded.UserData.Value)
	return string(raw)
}
