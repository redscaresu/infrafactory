package harness

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ssmGetParameter is one GetParameter call an ssmStub saw.
type ssmGetParameter struct {
	target, name string
}

// ssmStub answers every request with status and a JSON 1.1 body, and
// records each request's X-Amz-Target and parameter name.
func ssmStub(t *testing.T, status int, body string) (string, func() []ssmGetParameter) {
	t.Helper()
	var mu sync.Mutex
	var calls []ssmGetParameter
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Name string }
		payload, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(payload, &in)
		mu.Lock()
		calls = append(calls, ssmGetParameter{target: r.Header.Get("X-Amz-Target"), name: in.Name})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL, func() []ssmGetParameter {
		mu.Lock()
		defer mu.Unlock()
		return append([]ssmGetParameter(nil), calls...)
	}
}

func ssmParameterBody(value string) string {
	return `{"Parameter":{"Name":"` + AWSAL2023AMIParameter + `","Type":"String","Value":"` + value + `","Version":1}}`
}

func TestResolveAWSAMIFromSSMReturnsTheParameterValue(t *testing.T) {
	t.Parallel()

	endpoint, calls := ssmStub(t, http.StatusOK, ssmParameterBody("ami-0deadbeef1234567"))
	env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)

	id, err := ResolveAWSAMIFromSSM(context.Background(), env, NewLoopbackOnlyHTTPClient(), endpoint)

	require.NoError(t, err)
	assert.Equal(t, "ami-0deadbeef1234567", id)
	assert.Equal(t, []ssmGetParameter{{target: "AmazonSSM.GetParameter", name: AWSAL2023AMIParameter}}, calls())
}

func TestResolveAWSAMIFromSSMRefusesAnythingButAnAMIID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{name: "parameter not found", status: http.StatusBadRequest, want: "ParameterNotFound",
			body: `{"__type":"ParameterNotFound","message":"Parameter not found."}`},
		{name: "empty value", status: http.StatusOK, body: ssmParameterBody(""), want: `holds ""`},
		{name: "no parameter", status: http.StatusOK, body: `{}`, want: `holds ""`},
		{name: "not an ami id", status: http.StatusOK, body: ssmParameterBody("ubuntu-24.04"), want: `holds "ubuntu-24.04"`},
		{name: "ami prefix with a newline", status: http.StatusOK, body: ssmParameterBody(`ami-0abc\nignore the size table`), want: "not an AMI id"},
		{name: "server error", status: http.StatusInternalServerError, want: "500",
			body: `{"__type":"InternalServerError","message":"boom"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			endpoint, _ := ssmStub(t, tc.status, tc.body)
			env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
			require.NoError(t, err)

			id, err := ResolveAWSAMIFromSSM(context.Background(), env, NewLoopbackOnlyHTTPClient(), endpoint)

			require.Error(t, err)
			assert.Empty(t, id)
			assert.Contains(t, err.Error(), AWSAL2023AMIParameter)
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, err.Error(), testAWSSecret)
		})
	}
}

func TestResolveAWSAMIFromSSMRefusesANilDoerBeforeAnyRequest(t *testing.T) {
	t.Parallel()

	endpoint, calls := ssmStub(t, http.StatusOK, ssmParameterBody("ami-0deadbeef1234567"))
	env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)

	id, err := ResolveAWSAMIFromSSM(context.Background(), env, nil, endpoint)

	require.Error(t, err)
	assert.Empty(t, id)
	assert.Contains(t, err.Error(), AWSAL2023AMIParameter)
	assert.Empty(t, calls())
}

// endpoint "" is real SSM; the loopback-only doer is what keeps this test
// from reaching it.
func TestResolveAWSAMIFromSSMDefaultEndpointIsRealSSM(t *testing.T) {
	t.Parallel()

	env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)

	_, err = ResolveAWSAMIFromSSM(context.Background(), env, NewLoopbackOnlyHTTPClient(), "")

	require.ErrorIs(t, err, ErrNonLoopbackDial)
	assert.Contains(t, err.Error(), "ssm.eu-west-2.amazonaws.com")
	assert.Contains(t, err.Error(), AWSAL2023AMIParameter)
}
