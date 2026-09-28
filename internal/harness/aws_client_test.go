package harness

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAWSAccount           = "123456789012"
	getCallerIdentityXMLBody = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::123456789012:user/infrafactory-layer3</Arn>
    <UserId>AIDAEXAMPLEUSERID0001</UserId>
    <Account>123456789012</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata><RequestId>00000000-0000-0000-0000-000000000000</RequestId></ResponseMetadata>
</GetCallerIdentityResponse>`
)

// capturingDoer answers every request itself and never dials, so a test
// sees where a client would have sent a request without it going there.
type capturingDoer struct {
	requests []*http.Request
}

func (d *capturingDoer) Do(req *http.Request) (*http.Response, error) {
	d.requests = append(d.requests, req)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(getCallerIdentityXMLBody)),
		Request:    req,
	}, nil
}

// plantAWSTrap stands up a loopback server that counts requests, and a
// HOME whose ~/.aws files select a planted profile, key, region and a
// `services` STS endpoint pointing at the trap. It returns the trap URL,
// its request counter and the HOME.
func plantAWSTrap(t *testing.T) (string, *atomic.Int32, string) {
	t.Helper()
	var hits atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, getCallerIdentityXMLBody)
	}))
	t.Cleanup(trap.Close)

	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	require.NoError(t, os.MkdirAll(awsDir, 0o700))
	cfg := "[default]\nregion = ap-southeast-2\nservices = trap\n\n" +
		"[profile planted]\nregion = ap-southeast-2\nservices = trap\n\n" +
		"[services trap]\nsts =\n  endpoint_url = " + trap.URL + "\n"
	creds := "[default]\naws_access_key_id = PLANTEDDEFAULT\naws_secret_access_key = planted\n\n" +
		"[planted]\naws_access_key_id = PLANTEDPROFILE\naws_secret_access_key = planted\n"
	require.NoError(t, os.WriteFile(filepath.Join(awsDir, "config"), []byte(cfg), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(creds), 0o600))
	return trap.URL, &hits, home
}

func TestAWSSTSClientDefaultEndpointIgnoresPlantedEnvAndConfig(t *testing.T) {
	trapURL, hits, home := plantAWSTrap(t)
	t.Setenv("HOME", home)
	t.Setenv("AWS_ENDPOINT_URL_STS", trapURL)
	t.Setenv("AWS_ENDPOINT_URL", trapURL)
	t.Setenv("AWS_PROFILE", "planted")
	// The SDK read HOME at init, so name the planted files outright too.
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws", "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, ".aws", "credentials"))

	env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
	require.NoError(t, err)
	doer := &capturingDoer{}
	client, err := NewAWSSTSClient(env, doer, "")
	require.NoError(t, err)

	out, err := client.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	require.NoError(t, err)
	assert.Equal(t, testAWSAccount, aws.ToString(out.Account))

	require.Len(t, doer.requests, 1)
	assert.Equal(t, "https://sts.us-east-1.amazonaws.com/", doer.requests[0].URL.String())
	assert.Contains(t, doer.requests[0].Header.Get("Authorization"), "Credential="+testAWSKeyID+"/")
	assert.Zero(t, hits.Load(), "the planted endpoint was reached")
}

func TestAWSSTSClientLoopbackOnlyDoer(t *testing.T) {
	t.Parallel()

	var auth atomic.Value
	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, getCallerIdentityXMLBody)
	}))
	t.Cleanup(stsServer.Close)

	env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)

	client, err := NewAWSSTSClient(env, NewLoopbackOnlyHTTPClient(), stsServer.URL)
	require.NoError(t, err)
	out, err := client.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	require.NoError(t, err)
	assert.Equal(t, testAWSAccount, aws.ToString(out.Account))
	assert.Contains(t, auth.Load(), "Credential="+testAWSKeyID+"/")

	escaped, err := NewAWSSTSClient(env, NewLoopbackOnlyHTTPClient(), "https://203.0.113.1")
	require.NoError(t, err)
	_, err = escaped.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{},
		func(o *sts.Options) { o.RetryMaxAttempts = 1 })
	require.ErrorIs(t, err, ErrNonLoopbackDial)
}

func TestAWSSTSClientNilDoerKeepsTheSDKDefaultClient(t *testing.T) {
	t.Parallel()

	env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
	require.NoError(t, err)
	client, err := NewAWSSTSClient(env, nil, "")
	require.NoError(t, err)
	assert.NotNil(t, client.Options().HTTPClient)
}

func TestAWSSTSClientRefusesMalformedRegionBeforeAnyRequest(t *testing.T) {
	t.Parallel()

	env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
	require.NoError(t, err)

	for _, region := range []string{"", "us-east-1.evil.com", "x@evil.example/", "US-EAST-1"} {
		env["AWS_REGION"] = region
		doer := &capturingDoer{}
		client, err := NewAWSSTSClient(env, doer, "")
		require.Error(t, err, region)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "not an AWS region name")
		assert.Empty(t, doer.requests)
	}
}

// terraform-provider-aws builds on the SDK default chain, so tofu sees
// the sealed map through it. The two sentinel paths must read as absent
// (not as an error), and the planted HOME must lose to the map. The
// control run first proves the plant would win without the seal.
//
// Each load runs in a fresh process: the SDK fixes its ~/.aws paths from
// HOME at package init, as it does in tofu's provider, so an in-process
// HOME change would plant nothing.
func TestAWSSealedEnvThroughTheSDKDefaultChain(t *testing.T) {
	t.Parallel()
	trapURL, hits, home := plantAWSTrap(t)

	assert.Equal(t, "key=PLANTEDPROFILE region=ap-southeast-2 sts="+trapURL,
		runAWSChainProbe(t, map[string]string{"HOME": home, "AWS_PROFILE": "planted"}),
		"control: without the seal the plant must win, or the sealed run proves nothing")

	sealed, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)
	sealed["HOME"] = home
	assert.Equal(t, "key="+testAWSKeyID+" region=eu-west-2 sts=https://sts.eu-west-2.amazonaws.com",
		runAWSChainProbe(t, sealed))

	assert.Zero(t, hits.Load())
}

const awsChainProbeEnv = "INFRAFACTORY_AWS_CHAIN_PROBE"

// runAWSChainProbe re-runs this test binary as TestAWSDefaultChainProbe
// with exactly env, plus the probe marker, and returns what it resolved.
func runAWSChainProbe(t *testing.T, env map[string]string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAWSDefaultChainProbe$", "-test.count=1")
	cmd.Env = []string{awsChainProbeEnv + "=1"}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	for _, line := range strings.Split(string(out), "\n") {
		if resolved, ok := strings.CutPrefix(line, "aws-chain-probe: "); ok {
			return resolved
		}
	}
	require.FailNow(t, "the probe printed no result", string(out))
	return ""
}

func TestAWSDefaultChainProbe(t *testing.T) {
	if os.Getenv(awsChainProbeEnv) == "" {
		t.Skip("run in a fresh process by TestAWSSealedEnvThroughTheSDKDefaultChain")
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	creds, err := cfg.Credentials.Retrieve(ctx)
	require.NoError(t, err)

	doer := &capturingDoer{}
	_, err = sts.NewFromConfig(cfg, func(o *sts.Options) { o.HTTPClient = doer }).
		GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	require.NoError(t, err)
	require.Len(t, doer.requests, 1)
	endpoint := url.URL{Scheme: doer.requests[0].URL.Scheme, Host: doer.requests[0].URL.Host}

	fmt.Printf("aws-chain-probe: key=%s region=%s sts=%s\n", creds.AccessKeyID, cfg.Region, endpoint.String())
}
