package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const loopbackProviders = `provider "aws" {
  region = "us-east-1"
  endpoints {
    ec2 = "http://127.0.0.1:8082/ec2/region/us-east-1"
    s3  = "http://localhost:9091"
  }
}
`

// subnetApplyFailure is run 20260603T214517Z iteration 1's failure, cut down.
const subnetApplyFailure = "exit status 1 | stderr: Error: waiting for EC2 Subnet (subnet-a8) MapPublicIpOnLaunch update: timeout\n  with aws_subnet.a,"

// legacyRun lays out <root>/<scenario>/<run>/iterations/1/generated and
// returns the generated dir.
func legacyRun(t *testing.T, scenario, providers string, failure *string) string {
	t.Helper()
	iter := filepath.Join(t.TempDir(), scenario, "20260603T214517Z", "iterations", "1")
	gen := filepath.Join(iter, "generated")
	require.NoError(t, os.MkdirAll(gen, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gen, "network.tf"), []byte(subnetShape), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(gen, "providers.tf"), []byte(providers), 0o644))
	if failure != nil {
		payload, err := json.Marshal(map[string]any{"failures": []map[string]string{{"check": "apply", "detail": *failure}}})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(iter, "iteration.json"), payload, 0o644))
	}
	return gen
}

func TestEstablishLegacyLayer(t *testing.T) {
	failure := subnetApplyFailure
	attrs := []string{"map_public_ip_on_launch"}

	t.Run("established", func(t *testing.T) {
		evidence, err := EstablishLegacyLayer(legacyRun(t, "aws-eks", loopbackProviders, &failure), "aws-eks", "aws_subnet", attrs)
		require.NoError(t, err)
		assert.Contains(t, evidence, "run aws-eks/20260603T214517Z iteration 1")
	})

	noFailureAttr := "exit status 1 | stderr: Error: creating EC2 Subnet\n  with aws_subnet.a,"
	remote := strings.Replace(loopbackProviders, "http://127.0.0.1:8082", "https://ec2.us-east-1.amazonaws.com", 1)
	noEndpoints := "provider \"aws\" {\n  region = \"us-east-1\"\n}\n"
	for name, tc := range map[string]struct {
		from string
		want string
	}{
		"scenario mismatch":      {legacyRun(t, "aws-web", loopbackProviders, &failure), "not the entry's discovered_from"},
		"non-loopback endpoint":  {legacyRun(t, "aws-eks", remote, &failure), "not on a loopback host"},
		"no endpoints block":     {legacyRun(t, "aws-eks", noEndpoints, &failure), "no endpoints block"},
		"no provider block":      {legacyRun(t, "aws-eks", "", &failure), `no provider "aws" block`},
		"missing iteration.json": {legacyRun(t, "aws-eks", loopbackProviders, nil), "read iteration record"},
		"detail misses an attr":  {legacyRun(t, "aws-eks", loopbackProviders, &noFailureAttr), "no apply failure"},
		"not a run layout":       {t.TempDir(), "is not a run's"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := EstablishLegacyLayer(tc.from, "aws-eks", "aws_subnet", attrs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("every attribute must be named", func(t *testing.T) {
		_, err := EstablishLegacyLayer(legacyRun(t, "aws-eks", loopbackProviders, &failure), "aws-eks", "aws_subnet",
			[]string{"map_public_ip_on_launch", "assign_ipv6_address_on_creation"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no apply failure")
	})
}

func TestCutAvoidShapeKeepsTheBlocksAndWhatTheyReference(t *testing.T) {
	dir := t.TempDir()
	src := subnetShape + `
resource "aws_subnet" "private" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.2.0/24"
}

resource "aws_route_table" "rt" {
  vpc_id = aws_vpc.main.id
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644))

	shape, err := CutAvoidShape(dir, "aws_subnet", []string{"map_public_ip_on_launch"})
	require.NoError(t, err)

	cut := string(shape[AvoidShapeFile])
	assert.Equal(t, []string{AvoidShapeFile}, keys(shape))
	assert.Contains(t, cut, `resource "aws_subnet" "a"`)
	assert.Contains(t, cut, `resource "aws_vpc" "main"`, "the subnet references the vpc")
	assert.NotContains(t, cut, `"private"`, "a subnet that does not set the attribute is not the shape")
	assert.NotContains(t, cut, "aws_route_table", "nothing in the shape references it")
}

func TestCutAvoidShapeRefusesAShapeThatDoesNotExerciseTheRule(t *testing.T) {
	for name, tc := range map[string]struct {
		value string
		want  string
	}{
		"attribute missing": {"", "sets every attribute"},
		"literal false":     {"map_public_ip_on_launch = false", "literal off"},
		"literal null":      {"map_public_ip_on_launch = null", "literal off"},
		"empty string":      {`map_public_ip_on_launch = ""`, "literal off"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			src := "resource \"aws_subnet\" \"a\" {\n  cidr_block = \"10.0.1.0/24\"\n  " + tc.value + "\n}\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644))

			_, err := CutAvoidShape(dir, "aws_subnet", []string{"map_public_ip_on_launch"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestPrepareAvoidCheckEstablishesALegacyEntrysLayer(t *testing.T) {
	failure := subnetApplyFailure
	from := legacyRun(t, "aws-eks", loopbackProviders, &failure)
	legacy := subnetAvoidEntry()
	legacy.LearnedLayer = ""

	t.Run("established from the run", func(t *testing.T) {
		dir := t.TempDir()
		writeAWSCorpus(t, dir, legacy)
		prep, err := PrepareAvoidCheck(dir, "aws", from, "aws_subnet", []string{"map_public_ip_on_launch"})
		require.NoError(t, err)
		assert.Contains(t, prep.Retirement.LayerEvidence, "20260603T214517Z")
	})

	t.Run("not established", func(t *testing.T) {
		dir := t.TempDir()
		writeAWSCorpus(t, dir, legacy)
		_, err := PrepareAvoidCheck(dir, "aws", t.TempDir(), "aws_subnet", []string{"map_public_ip_on_launch"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "layer is not established")
	})

	t.Run("a malformed ledger", func(t *testing.T) {
		dir := t.TempDir()
		writeAWSCorpus(t, dir, subnetAvoidEntry())
		require.NoError(t, os.MkdirAll(avoidChecksDir(dir), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(avoidChecksDir(dir), "aws.yaml"), []byte("provider: aws\nrecords: [{status: bogus}]\n"), 0o644))
		_, err := PrepareAvoidCheck(dir, "aws", from, "aws_subnet", []string{"map_public_ip_on_launch"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ledger unreadable")
	})
}

func TestClassifyAvoidFailure(t *testing.T) {
	attrs := []string{"map_public_ip_on_launch"}
	assert.Equal(t, AvoidOutcomeRecurred, ClassifyAvoidFailure("MapPublicIpOnLaunch update: timeout", "aws_subnet", attrs))
	assert.Equal(t, AvoidOutcomeRecurred, ClassifyAvoidFailure("with aws_subnet.a,", "aws_subnet", attrs))
	assert.Equal(t, AvoidOutcomeInconclusive, ClassifyAvoidFailure("creating EC2 VPC: connection refused", "aws_subnet", attrs))
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
