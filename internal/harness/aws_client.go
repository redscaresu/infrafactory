package harness

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// NewAWSSTSClient builds infrafactory's in-process STS client from the
// AWSSealedEnv map alone, with static credentials and the endpoint
// https://sts.<region>.amazonaws.com. endpoint, when non-empty, replaces
// that default and is for tests. doer carries every request; nil means
// the SDK's default HTTP client.
//
// Not config.LoadDefaultConfig. The default chain reads
// AWS_ENDPOINT_URL_* and AWS_PROFILE from the process env, the profile's
// `services` endpoint overrides from ~/.aws/config, and credentials from
// IMDS. SandboxStripEnv removes AWS_* only from child processes, so the
// parent's own env still holds whatever the shell exported, and any of
// those would send this client somewhere other than real AWS. A test
// audits that no non-test code imports the config module.
func NewAWSSTSClient(env map[string]string, doer sts.HTTPClient, endpoint string) (*sts.Client, error) {
	region := env["AWS_REGION"]
	if !ValidAWSRegion(region) {
		return nil, fmt.Errorf("aws sts client: region %q is not an AWS region name", region)
	}
	keyID, secret := env["AWS_ACCESS_KEY_ID"], env["AWS_SECRET_ACCESS_KEY"]
	if keyID == "" || secret == "" {
		return nil, errors.New("aws sts client: the sealed env has no AWS_ACCESS_KEY_ID or AWS_SECRET_ACCESS_KEY")
	}
	if endpoint == "" {
		endpoint = "https://sts." + region + ".amazonaws.com"
	}

	cfg := aws.Config{
		Region:      region,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(keyID, secret, "")),
	}
	return sts.NewFromConfig(cfg, func(o *sts.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		// sts.New resolves its default client before running options, so
		// assigning a nil doer would erase it.
		if doer != nil {
			o.HTTPClient = doer
		}
	}), nil
}

// ErrNonLoopbackDial is returned by NewLoopbackOnlyHTTPClient's dialer.
var ErrNonLoopbackDial = errors.New("refusing to dial a non-loopback address")

// NewLoopbackOnlyHTTPClient is the doer for tests of AWS clients: its
// dialer refuses every address that is not a loopback IP, so a request
// that escapes the test's endpoint fails instead of reaching AWS. It
// uses no proxy; a proxy env var cannot help, because
// http.ProxyFromEnvironment caches once per process and never proxies
// loopback.
func NewLoopbackOnlyHTTPClient() *http.Client {
	var dialer net.Dialer
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
				return nil, fmt.Errorf("%w: %s", ErrNonLoopbackDial, addr)
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}}
}
