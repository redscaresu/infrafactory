package harness

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAWSPrincipal   = "arn:aws:iam::123456789012:user/infrafactory-layer3"
	stsInvalidTokenXML = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error><Type>Sender</Type><Code>InvalidClientTokenId</Code><Message>The security token included in the request is invalid.</Message></Error>
  <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
</ErrorResponse>`
)

// stsStub answers every request with status and body, counting requests
// and keeping the last Authorization header.
func stsStub(t *testing.T, status int, body string) (string, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var hits atomic.Int32
	var auth atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		auth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL, &hits, &auth
}

func TestVerifyAWSIdentityPassesOnlyTheConfiguredAccountAndPrincipal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		status        int
		body          string
		wantAccount   string
		wantPrincipal string
		wantErr       []string
	}{
		{name: "match", status: http.StatusOK, body: getCallerIdentityXMLBody,
			wantAccount: testAWSAccount, wantPrincipal: testAWSPrincipal},
		{name: "wrong account", status: http.StatusOK, body: getCallerIdentityXMLBody,
			wantAccount: "210987654321", wantPrincipal: testAWSPrincipal,
			wantErr: []string{`account "123456789012"`, `configured account "210987654321"`}},
		{name: "wrong principal", status: http.StatusOK, body: getCallerIdentityXMLBody,
			wantAccount: testAWSAccount, wantPrincipal: "arn:aws:iam::123456789012:user/admin",
			wantErr: []string{`principal "` + testAWSPrincipal + `"`, `configured principal "arn:aws:iam::123456789012:user/admin"`}},
		{name: "sts 403", status: http.StatusForbidden, body: stsInvalidTokenXML,
			wantAccount: testAWSAccount, wantPrincipal: testAWSPrincipal,
			wantErr: []string{"sts:GetCallerIdentity failed", "403", "InvalidClientTokenId"}},
		{name: "malformed xml", status: http.StatusOK, body: "<GetCallerIdentityResponse><GetCallerIdentityResult><Account>",
			wantAccount: testAWSAccount, wantPrincipal: testAWSPrincipal,
			wantErr: []string{"sts:GetCallerIdentity failed", "deserialization failed"}},
		// A body with no identity must not satisfy an empty expectation.
		{name: "empty result", status: http.StatusOK,
			body:        `<GetCallerIdentityResponse><GetCallerIdentityResult></GetCallerIdentityResult></GetCallerIdentityResponse>`,
			wantAccount: testAWSAccount, wantPrincipal: testAWSPrincipal,
			wantErr: []string{`account ""`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			endpoint, hits, auth := stsStub(t, tc.status, tc.body)
			env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
			require.NoError(t, err)

			err = VerifyAWSIdentity(context.Background(), env, NewLoopbackOnlyHTTPClient(), endpoint, tc.wantAccount, tc.wantPrincipal)

			require.NotZero(t, hits.Load())
			assert.Contains(t, auth.Load(), "Credential="+testAWSKeyID+"/")
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tc.wantErr {
				assert.Contains(t, err.Error(), want)
			}
			assert.NotContains(t, err.Error(), testAWSSecret)
		})
	}
}

func TestVerifyAWSIdentityRefusesAnEmptyExpectationBeforeAnyRequest(t *testing.T) {
	t.Parallel()

	endpoint, hits, _ := stsStub(t, http.StatusOK, getCallerIdentityXMLBody)
	env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)

	for _, want := range [][2]string{{"", testAWSPrincipal}, {testAWSAccount, ""}} {
		err := VerifyAWSIdentity(context.Background(), env, NewLoopbackOnlyHTTPClient(), endpoint, want[0], want[1])
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must both be set")
	}
	assert.Zero(t, hits.Load())
}
