package e2e

import (
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every service fakeaws serves without Docker, applied through the
// provider block generation writes, which names no endpoint: each call
// reaches fakeaws through its AWS_ENDPOINT_URL_<SERVICE> in cloudEnv, and
// STS is called at provider configure. A misspelt variable sends that
// service to the dead catch-all and the apply fails. S3 is
// TestE2E_AWSFullStack's (Docker).
func TestE2E_AWSEnvOnlyEveryService(t *testing.T) {
	run, mock := startSealedAWSFixture(t, "aws-env-only", "every-service")
	collections := [][2]string{
		{"ec2", "vpcs"}, {"ec2", "subnets"}, {"iam", "roles"}, {"sqs", "queues"},
		{"dynamodb", "tables"}, {"route53", "hosted_zones"}, {"secretsmanager", "secrets"},
		{"rds", "db_subnet_groups"}, {"rds", "db_parameter_groups"}, {"eks", "clusters"},
	}

	// A pass means the second plan was empty: drift fails the run.
	applied := run("--no-destroy")
	require.NoError(t, applied.Err, "stdout:\n%s\nfakeaws log: %s", applied.Stdout, mock.LogPath())
	assert.Contains(t, applied.Stdout, "run/terminal_reason: pass (target_reached)")
	state := mock.FetchState(t)
	for _, c := range collections {
		assert.NotZero(t, awsStateItemCount(state, c[0], c[1]), "%s/%s after apply", c[0], c[1])
	}
	assertTaggedWithRunID(t, mock.URL, onlyRunID(t, "aws-env-only-every-service"), state)

	destroyed := run()
	require.NoError(t, destroyed.Err, "stdout:\n%s\nfakeaws log: %s", destroyed.Stdout, mock.LogPath())
	assert.Contains(t, destroyed.Stdout, "run/terminal_reason: pass (target_reached)")
	state = mock.FetchState(t)
	for _, c := range collections {
		assert.Zero(t, awsStateItemCount(state, c[0], c[1]), "%s/%s after destroy", c[0], c[1])
	}
}

// With no skip_* in the provider block, the provider calls STS when it
// configures, so `validate` needs fakeaws up. Down, plan fails dialing
// fakeaws's address: the dead proxy instead would mean STS went to real
// AWS.
func TestE2E_AWSValidateNeedsFakeawsSTS(t *testing.T) {
	SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	closed := fmt.Sprintf("127.0.0.1:%d", pickFreePort(t))
	SealNetwork(t)
	command := sealedAWSCommand(t, "aws-layer2-env", "vpc-subnet", "http://"+closed)

	generated := command("generate")
	require.NoError(t, generated.Err, "stdout:\n%s\nstderr:\n%s", generated.Stdout, generated.Stderr)
	result := command("validate")

	require.Error(t, result.Err, "stdout:\n%s", result.Stdout)
	output := result.Stdout + result.Stderr
	assert.Contains(t, output, "static/plan: fail", output)
	assert.Contains(t, output, "dial tcp "+closed+":", output)
	assert.NotContains(t, output, "dial tcp 127.0.0.1:9:")
}

// onlyRunID is the id of the one run the run store holds for scenario.
func onlyRunID(t *testing.T, scenario string) string {
	t.Helper()
	runs, err := os.ReadDir(filepath.Join(os.Getenv(envRunStoreRoot), scenario))
	require.NoError(t, err)
	require.Len(t, runs, 1)
	return runs[0].Name()
}

// assertTaggedWithRunID reads one resource's tags on each service the run
// id must reach back through that service's own tag call on fakeaws: the
// provider's default_tags, stored at create and returned on read.
func assertTaggedWithRunID(t *testing.T, fakeawsURL, runID string, state map[string]any) {
	t.Helper()
	route53, _ := state["route53"].(map[string]any)
	zones, _ := route53["hosted_zones"].([]any)
	require.Len(t, zones, 1)
	zone, _ := zones[0].(map[string]any)
	zoneID, _ := zone["id"].(string)
	zoneID = strings.TrimPrefix(zoneID, "/hostedzone/")
	require.NotEmpty(t, zoneID, "route53 hosted zone id in %v", zones[0])
	const account, region = "000000000000", "us-east-1"
	queryRPC := "application/x-www-form-urlencoded"

	reads := map[string]func() (*http.Response, []byte){
		"sqs": func() (*http.Response, []byte) {
			return awsPostWithTargetJSON10(t, fakeawsURL+"/sqs/region/"+region, "AmazonSQS.ListQueueTags",
				`{"QueueUrl":"`+fakeawsURL+"/"+account+`/env-only-jobs"}`)
		},
		"iam": func() (*http.Response, []byte) {
			return awsPost(t, fakeawsURL+"/iam", "Action=ListRoleTags&Version=2010-05-08&RoleName=env-only-app", queryRPC)
		},
		"rds": func() (*http.Response, []byte) {
			return awsPost(t, fakeawsURL+"/rds/region/"+region, "Action=ListTagsForResource&Version=2014-10-31&ResourceName="+
				neturl.QueryEscape("arn:aws:rds:"+region+":"+account+":subgrp:env-only-db"), queryRPC)
		},
		"route53": func() (*http.Response, []byte) {
			return awsGet(t, fakeawsURL+"/route53/2013-04-01/tags/hostedzone/"+zoneID)
		},
		"dynamodb": func() (*http.Response, []byte) {
			return awsPostWithTarget(t, fakeawsURL+"/dynamodb/region/"+region, "DynamoDB_20120810.ListTagsOfResource",
				`{"ResourceArn":"arn:aws:dynamodb:`+region+":"+account+`:table/env-only-items"}`)
		},
		"eks": func() (*http.Response, []byte) {
			return awsGet(t, fakeawsURL+"/eks/region/"+region+"/tags/"+
				neturl.PathEscape("arn:aws:eks:"+region+":"+account+":cluster/env-only-cluster"))
		},
	}
	for service, read := range reads {
		resp, body := read()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", service, body)
		assert.Contains(t, string(body), "infrafactory-run-id", service)
		assert.Contains(t, string(body), runID, service)
	}
}
