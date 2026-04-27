package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// AWS gated e2e tests. Per concepts.md "Required surface" item 11:
// these mirror runGCPServiceScenario but target fakeaws via the
// hashicorp/aws provider. Currently exercised through the
// /mock/state lifecycle — full tofu apply→update→destroy lands as
// service handlers + scenarios mature in S44+.
//
// Today (S43-T14) the tests:
//   - Boot fakeaws via StartFakeaws (the helper added in S43-T9).
//   - Hit the mock's /healthz to confirm reachability.
//   - Issue a Query-RPC CreateRole (TestE2E_AWS_IAM) or path-style
//     PutBucket (TestE2E_AWS_S3) directly.
//   - Snapshot the response/state, apply an update, verify identity
//     preservation: the resource's name + ARN are byte-identical
//     pre/post update. This is the key contract — destroy+recreate
//     would change the ARN, and the run-loop's drift detection
//     would catch it.
//   - Tear down via /mock/reset.
//
// Gated by SkipUnlessEnabled (INFRAFACTORY_ENABLE_E2E=1). Without the
// env var, the tests skip cleanly.

func TestE2E_AWS_IAM(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("go binary required: %v", err)
	}
	mock := StartFakeaws(t)

	// Create role.
	body := url("Action=CreateRole&Version=2010-05-08&RoleName=e2e-role&AssumeRolePolicyDocument=" +
		urlEncode(`{"Version":"2012-10-17"}`) + "&Description=initial")
	resp, respBody := awsPost(t, mock.URL+"/iam", body, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateRole: %d body=%s", resp.StatusCode, string(respBody))
	}
	roleARNBefore := extractTagValue(string(respBody), "Arn")
	if !strings.HasPrefix(roleARNBefore, "arn:aws:iam::") {
		t.Fatalf("expected ARN in CreateRole response, got %q (body=%s)", roleARNBefore, respBody)
	}

	// Update description.
	updateBody := url("Action=UpdateRole&Version=2010-05-08&RoleName=e2e-role&Description=updated")
	resp, _ = awsPost(t, mock.URL+"/iam", updateBody, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("UpdateRole: %d", resp.StatusCode)
	}

	// Identity preservation: GetRole returns the SAME ARN.
	getBody := url("Action=GetRole&Version=2010-05-08&RoleName=e2e-role")
	resp, respBody = awsPost(t, mock.URL+"/iam", getBody, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GetRole: %d", resp.StatusCode)
	}
	roleARNAfter := extractTagValue(string(respBody), "Arn")
	if roleARNAfter != roleARNBefore {
		t.Errorf("identity preservation failed: ARN changed from %q to %q across update — destroy+recreate detected",
			roleARNBefore, roleARNAfter)
	}
	desc := extractTagValue(string(respBody), "Description")
	if desc != "updated" {
		t.Errorf("Description: got %q want updated", desc)
	}

	// /mock/state surfaces the role.
	state := mock.FetchState(t)
	stateBytes, _ := json.Marshal(state)
	if !strings.Contains(string(stateBytes), `"e2e-role"`) {
		t.Errorf("/mock/state missing role: %s", stateBytes)
	}

	// Reset cleanup.
	mock.Reset(t)
}

func TestE2E_AWS_S3(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)

	// PutBucket.
	resp, _ := awsPost(t, mock.URL+"/s3/e2e-bucket/", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PutBucket: %d", resp.StatusCode)
	}

	// PutBucketVersioning Enabled.
	versioningEnabled := `<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>`
	resp, _ = awsPut(t, mock.URL+"/s3/e2e-bucket/?versioning", versioningEnabled, "application/xml")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PutBucketVersioning: %d", resp.StatusCode)
	}

	// Snapshot bucket name from /mock/state pre-update.
	stateBefore := mock.FetchState(t)
	stateBeforeBytes, _ := json.Marshal(stateBefore)
	if !strings.Contains(string(stateBeforeBytes), `"e2e-bucket"`) {
		t.Fatalf("bucket missing from state pre-update: %s", stateBeforeBytes)
	}

	// Update versioning Suspended (in-place flip).
	versioningSuspended := `<VersioningConfiguration><Status>Suspended</Status></VersioningConfiguration>`
	resp, _ = awsPut(t, mock.URL+"/s3/e2e-bucket/?versioning", versioningSuspended, "application/xml")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PutBucketVersioning(Suspended): %d", resp.StatusCode)
	}

	// Identity preservation: bucket still exists with the same name +
	// region. GET versioning returns the new value.
	stateAfter := mock.FetchState(t)
	stateAfterBytes, _ := json.Marshal(stateAfter)
	if !strings.Contains(string(stateAfterBytes), `"e2e-bucket"`) {
		t.Errorf("identity preservation failed: bucket gone from state post-update: %s", stateAfterBytes)
	}

	getResp, getBody := awsGet(t, mock.URL+"/s3/e2e-bucket/?versioning")
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GetBucketVersioning: %d", getResp.StatusCode)
	}
	if !strings.Contains(string(getBody), "Suspended") {
		t.Errorf("versioning not flipped: %s", getBody)
	}

	mock.Reset(t)
}

// ----- minimal helpers (aws-side) -----

func awsPost(t *testing.T, url, body, contentType string) (*http.Response, []byte) {
	t.Helper()
	return awsRequest(t, http.MethodPost, url, body, contentType)
}

func awsPut(t *testing.T, url, body, contentType string) (*http.Response, []byte) {
	t.Helper()
	return awsRequest(t, http.MethodPut, url, body, contentType)
}

func awsGet(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	return awsRequest(t, http.MethodGet, url, "", "")
}

func awsRequest(t *testing.T, method, url, body, contentType string) (*http.Response, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var req *http.Request
	var err error
	if body != "" {
		req, err = http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, _ := readAllBytes(resp.Body)
	return resp, respBody
}

func readAllBytes(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var out []byte
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return out, nil
}

// extractTagValue is a tiny XML peek that pulls the first occurrence
// of <Tag>value</Tag>. Avoids dragging encoding/xml into the e2e
// helpers for this trivial use.
func extractTagValue(body, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(body, open)
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i+len(open):], close)
	if j < 0 {
		return ""
	}
	return body[i+len(open) : i+len(open)+j]
}

// url is a no-op alias for readability — body strings are already
// url-encoded by the caller.
func url(s string) string { return s }

// urlEncode percent-encodes an opaque string for a Query-RPC body
// parameter. We only need a tiny subset (curly braces, quotes, etc.).
func urlEncode(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '~':
			b.WriteRune(r)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}

// silence "imported and not used" for json (reserved for future
// state-shape assertions).
var _ = json.Valid
