package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestAWSReadAgainstFakeaws runs the account read through its walls over
// the loopback-only client against fakeaws STS and SSM, with the real
// identity check, so a change on either side of the wire fails here
// rather than against real AWS. fakeaws does not model DryRun, so the
// preflight is a stub. EC2, whose answers bring the follow-ups, is
// TestAWSReadEC2AgainstFakeaws. The required CI job runs it by name and fails if
// it does not report PASS.
func TestAWSReadAgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "eu-west-2"
	ctx := context.Background()

	credFile := filepath.Join(t.TempDir(), "reader-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=fakeaws-key-id\nAWS_SECRET_ACCESS_KEY=fakeaws-secret\n"), 0o600))
	cfg, err := harness.AWSReaderConfig(credFile, region)
	require.NoError(t, err)

	// Planted unwalled: the read itself sends only reader-table actions.
	creds := credentials.NewStaticCredentialsProvider("fakeaws-key-id", "fakeaws-secret", "")
	doer := harness.NewLoopbackOnlyHTTPClient()
	ssmClient := ssm.New(ssm.Options{Region: region, Credentials: creds, BaseEndpoint: aws.String(mock.URL + "/ssm/region/" + region), HTTPClient: doer})
	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name: aws.String("/app/db/host"), Value: aws.String("db.internal"), Type: ssmtypes.ParameterTypeString,
	})
	require.NoError(t, err)

	entry := func(service, protocol, action, params string) harness.AWSReadEntry {
		return harness.AWSReadEntry{Service: service, Protocol: protocol, Action: action, Params: json.RawMessage(params)}
	}
	readSet := []harness.AWSReadEntry{
		entry("sts", "query", "GetCallerIdentity", `{}`),
		entry("ssm", "json", "GetParameter", `{"Name":"/app/db/host"}`),
		entry("ssm", "json", "GetParameter", `{"Name":"/app/missing"}`),
	}
	read, err := harness.ReadAWSAccount(ctx, harness.AWSReadInput{
		Config: cfg, Account: fakeawsAccount, Principal: fakeawsPrincipal,
		ReadSet: readSet, Doer: doer,
		Identity:  harness.VerifyAWSReadIdentity,
		Preflight: func(context.Context, aws.Config, *harness.AWSReadWall) error { return nil },
		Endpoints: map[string]string{
			"sts": mock.URL + "/sts",
			"ssm": mock.URL + "/ssm/region/" + region,
			"ec2": mock.URL + "/ec2/region/" + region,
			"iam": mock.URL + "/iam",
		},
	})
	require.NoError(t, err)

	assert.Equal(t, fakeawsAccount, read.Account)
	assert.Equal(t, region, read.Region)
	assert.Equal(t, 1+len(readSet), read.Calls, "the identity check and each entry, once")
	require.Len(t, read.Exchanges, len(readSet))
	for i, exchange := range read.Exchanges {
		assert.Equal(t, readSet[i], exchange.AWSReadEntry, "exchange %d holds its entry verbatim", i)
	}

	identity, param, missing := read.Exchanges[0], read.Exchanges[1], read.Exchanges[2]
	assert.Equal(t, http.StatusOK, identity.Status)
	assert.Contains(t, identity.Body, "<Account>"+fakeawsAccount+"</Account>")

	assert.Equal(t, http.StatusOK, param.Status)
	var got struct{ Parameter struct{ Name, Value string } }
	require.NoError(t, json.Unmarshal([]byte(param.Body), &got), param.Body)
	assert.Equal(t, "db.internal", got.Parameter.Value)

	assert.Equal(t, http.StatusBadRequest, missing.Status, "a lookup that finds nothing is an answer")
	assert.Contains(t, missing.Body, "ParameterNotFound")
}
