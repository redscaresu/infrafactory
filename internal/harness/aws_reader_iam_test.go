package harness

import (
	"cmp"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const iamNamespace = "https://iam.amazonaws.com/doc/2010-05-08/"

// iamRecordingFake answers each IAM request with answer(form), and
// records every request as it reached the wire.
type iamRecordingFake struct {
	answer func(form url.Values) awsCannedAnswer
	sent   []awsWireRequest
}

func (f *iamRecordingFake) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	f.sent = append(f.sent, awsWireRequest{host: req.URL.Host, header: req.Header.Clone(), body: string(body)})
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	answer := f.answer(form)
	return &http.Response{
		StatusCode: cmp.Or(answer.status, http.StatusOK),
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(answer.body)),
		Request:    req,
	}, nil
}

// readIAMFake reads readSet with fake answering IAM.
func readIAMFake(t *testing.T, fake *iamRecordingFake, readSet ...AWSReadEntry) (AWSRecordedRead, error) {
	t.Helper()
	doer := newAWSReadTestDoers().doer()
	doer[awsReadIAMHost] = fake
	return ReadAWSAccount(context.Background(), awsReadTestInput(doer, readSet...))
}

func iamForm(action string, params map[string]string) url.Values {
	form := url.Values{"Action": {action}, "Version": {"2010-05-08"}}
	for k, v := range params {
		form.Set(k, v)
	}
	return form
}

func iamRoleBody(role string) string {
	return `<GetRoleResponse xmlns="` + iamNamespace + `"><GetRoleResult><Role><Path>/</Path><RoleName>` + role +
		`</RoleName><Arn>arn:aws:iam::123456789012:role/` + role + `</Arn></Role></GetRoleResult></GetRoleResponse>`
}

const iamAttachedBody = `<ListAttachedRolePoliciesResponse xmlns="` + iamNamespace + `"><ListAttachedRolePoliciesResult><AttachedPolicies><member>` +
	`<PolicyName>ReadOnlyAccess</PolicyName><PolicyArn>arn:aws:iam::aws:policy/ReadOnlyAccess</PolicyArn></member></AttachedPolicies>` +
	`<IsTruncated>false</IsTruncated></ListAttachedRolePoliciesResult></ListAttachedRolePoliciesResponse>`

// iamRoleAnswers answers as IAM does for role web: two inline policies
// over two ListRolePolicies pages (the fixtures), one attached policy,
// no instance profile. listPage1 is the first ListRolePolicies page.
func iamRoleAnswers(listPage1, listPage2 string) func(url.Values) awsCannedAnswer {
	return func(form url.Values) awsCannedAnswer {
		switch form.Get("Action") {
		case "GetRole":
			return awsOK(iamRoleBody(form.Get("RoleName")))
		case "ListRolePolicies":
			if form.Has("Marker") {
				return awsOK(listPage2)
			}
			return awsOK(listPage1)
		case "GetRolePolicy":
			return awsOK(`<GetRolePolicyResponse xmlns="` + iamNamespace + `"><GetRolePolicyResult><RoleName>` + form.Get("RoleName") +
				`</RoleName><PolicyName>` + form.Get("PolicyName") + `</PolicyName><PolicyDocument>%7B%7D</PolicyDocument></GetRolePolicyResult></GetRolePolicyResponse>`)
		case "ListAttachedRolePolicies":
			return awsOK(iamAttachedBody)
		case "ListInstanceProfilesForRole":
			return awsOK(`<ListInstanceProfilesForRoleResponse xmlns="` + iamNamespace + `"><ListInstanceProfilesForRoleResult><InstanceProfiles/>` +
				`<IsTruncated>false</IsTruncated></ListInstanceProfilesForRoleResult></ListInstanceProfilesForRoleResponse>`)
		}
		return awsCannedAnswer{status: http.StatusBadRequest, body: awsQueryErrorBody(iamNamespace, "InvalidAction")}
	}
}

func TestReadAWSAccountReadsARolesPolicies(t *testing.T) {
	t.Parallel()

	fake := &iamRecordingFake{answer: iamRoleAnswers(awsPageFixture(t, "iam", "ListRolePolicies", 1), awsPageFixture(t, "iam", "ListRolePolicies", 2))}

	read, err := readIAMFake(t, fake, readEntry("iam", "query", "GetRole", `{"RoleName":"web"}`))
	require.NoError(t, err)

	web := map[string]string{"RoleName": "web"}
	want := []url.Values{
		iamForm("GetRole", web),
		iamForm("ListRolePolicies", web),
		iamForm("ListRolePolicies", map[string]string{"RoleName": "web", "Marker": iamPageMarker2}),
		iamForm("GetRolePolicy", map[string]string{"RoleName": "web", "PolicyName": "CloudwatchPutMetricPolicy"}),
		iamForm("GetRolePolicy", map[string]string{"RoleName": "web", "PolicyName": "S3AccessPolicy"}),
		iamForm("ListAttachedRolePolicies", web),
		iamForm("ListInstanceProfilesForRole", web),
	}
	require.Len(t, fake.sent, len(want))
	for i, sent := range fake.sent {
		assert.Equal(t, want[i], sent.form(t), "request %d", i)
		assert.Equal(t, awsReadIAMHost, sent.host, "request %d", i)
		assertSignedFor(t, sent, "us-east-1", "iam")
	}

	require.Len(t, read.Exchanges, len(want), "every call is recorded")
	for i, exchange := range read.Exchanges {
		assert.Equal(t, want[i].Get("Action"), exchange.Action, "exchange %d", i)
		assert.Equal(t, http.StatusOK, exchange.Status, "exchange %d", i)
	}
	assert.Equal(t, iamAttachedBody, read.Exchanges[5].Body, "the attached policy is recorded raw")
	assert.Equal(t, 1+len(want), read.Calls, "identity and each IAM request, once")
}

func TestReadAWSAccountFailsATruncatedRoleListWithoutAMarker(t *testing.T) {
	t.Parallel()

	page1 := strings.Replace(awsPageFixture(t, "iam", "ListRolePolicies", 1), "<Marker>"+iamPageMarker2+"</Marker>", "", 1)
	fake := &iamRecordingFake{answer: iamRoleAnswers(page1, "")}

	read, err := readIAMFake(t, fake, readEntry("iam", "query", "GetRole", `{"RoleName":"web"}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `role "web": follow-up ListRolePolicies {"RoleName":"web"}`)
	assert.Contains(t, err.Error(), "IsTruncated is true with 0 Markers, not one")
	assert.Empty(t, read.Exchanges)
	assert.Len(t, fake.sent, 2, "nothing after the truncated page")
}
