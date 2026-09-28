package harness

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testEC2InstanceID = "i-0123456789abcdef0"
	testEC2AMIID      = "ami-0123456789abcdef0"

	// goldenWebStepOneUserData mirrors renderAWSUserData's output for the
	// web-step-one service, as internal/e2e/aws_web_step_one_test.go does.
	goldenWebStepOneUserData = `#!/bin/bash
# Rendered by infrafactory from the scenario's service: block.
set -euo pipefail
dnf install -y docker
systemctl enable --now docker
docker run -d --restart=always -p 80:80 'nginx:1.27'
`
	ec2UnauthorizedBody = `<Response><Errors><Error><Code>UnauthorizedOperation</Code>` +
		`<Message>You are not authorized to perform this operation.</Message></Error></Errors>` +
		`<RequestID>00000000-0000-0000-0000-000000000000</RequestID></Response>`
)

func ec2UserDataBody(userData string) string {
	return `<DescribeInstanceAttributeResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
		`<instanceId>` + testEC2InstanceID + `</instanceId>` + userData +
		`</DescribeInstanceAttributeResponse>`
}

func ec2ImagesBody(images ...string) string {
	return `<DescribeImagesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><imagesSet>` +
		strings.Join(images, "") + `</imagesSet></DescribeImagesResponse>`
}

func ec2Image(id, rootType, rootName, mappings string) string {
	return `<item><imageId>` + id + `</imageId><rootDeviceType>` + rootType + `</rootDeviceType>` +
		`<rootDeviceName>` + rootName + `</rootDeviceName><blockDeviceMapping>` + mappings +
		`</blockDeviceMapping></item>`
}

const ec2GP3RootMapping = `<item><deviceName>/dev/xvda</deviceName><ebs><volumeSize>8</volumeSize>` +
	`<volumeType>gp3</volumeType><deleteOnTermination>true</deleteOnTermination></ebs></item>`

// ec2CapturingDoer is capturingDoer answering with body instead, and
// keeping each request's query-protocol form.
type ec2CapturingDoer struct {
	capturingDoer
	body  string
	forms []url.Values
}

func (d *ec2CapturingDoer) Do(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(payload))
	if err != nil {
		return nil, err
	}
	d.forms = append(d.forms, form)
	resp, err := d.capturingDoer.Do(req)
	resp.Body = io.NopCloser(strings.NewReader(d.body))
	return resp, err
}

// ec2Stub answers every request with status and body, and records each
// request's form.
func ec2Stub(t *testing.T, status int, body string) (string, func() []url.Values) {
	t.Helper()
	var mu sync.Mutex
	var forms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		forms = append(forms, r.PostForm)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL, func() []url.Values {
		mu.Lock()
		defer mu.Unlock()
		return append([]url.Values(nil), forms...)
	}
}

// plantEC2Trap points every env route to an EC2 endpoint at plantAWSTrap's
// server, so a client that honoured any of them would reach it.
func plantEC2Trap(t *testing.T) func() int32 {
	t.Helper()
	trapURL, hits, home := plantAWSTrap(t)
	t.Setenv("HOME", home)
	t.Setenv("AWS_ENDPOINT_URL_EC2", trapURL)
	t.Setenv("AWS_ENDPOINT_URL", trapURL)
	t.Setenv("AWS_PROFILE", "planted")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws", "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, ".aws", "credentials"))
	return hits.Load
}

func assertSealedEC2Request(t *testing.T, doer *ec2CapturingDoer, want url.Values) {
	t.Helper()
	require.Len(t, doer.requests, 1)
	assert.Equal(t, "https://ec2.us-east-1.amazonaws.com/", doer.requests[0].URL.String())
	assert.Contains(t, doer.requests[0].Header.Get("Authorization"), "Credential="+testAWSKeyID+"/")
	want.Set("Version", "2016-11-15")
	assert.Equal(t, want, doer.forms[0])
}

func TestAWSInstanceUserDataDefaultEndpointIgnoresPlantedEnv(t *testing.T) {
	trapHits := plantEC2Trap(t)
	env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString([]byte(goldenWebStepOneUserData))
	doer := &ec2CapturingDoer{body: ec2UserDataBody(`<userData><value>` + encoded + `</value></userData>`)}

	got, err := AWSInstanceUserData(context.Background(), env, doer, "", testEC2InstanceID)

	require.NoError(t, err)
	assert.Equal(t, goldenWebStepOneUserData, string(got))
	assertSealedEC2Request(t, doer, url.Values{
		"Action": {"DescribeInstanceAttribute"}, "InstanceId": {testEC2InstanceID}, "Attribute": {"userData"},
	})
	assert.Zero(t, trapHits(), "the planted endpoint was reached")
}

func TestDescribeAWSAMIRootDefaultEndpointIgnoresPlantedEnv(t *testing.T) {
	trapHits := plantEC2Trap(t)
	env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
	require.NoError(t, err)
	doer := &ec2CapturingDoer{body: ec2ImagesBody(ec2Image(testEC2AMIID, "ebs", "/dev/xvda", ec2GP3RootMapping))}

	got, err := DescribeAWSAMIRoot(context.Background(), env, doer, "", testEC2AMIID)

	require.NoError(t, err)
	assert.Equal(t, AWSAMIRoot{SizeGiB: 8, VolumeType: "gp3", DeleteOnTermination: true}, got)
	assertSealedEC2Request(t, doer, url.Values{"Action": {"DescribeImages"}, "ImageId.1": {testEC2AMIID}})
	assert.Zero(t, trapHits(), "the planted endpoint was reached")
}

func TestAWSInstanceUserDataAgainstAFakeEC2(t *testing.T) {
	t.Parallel()
	encoded := base64.StdEncoding.EncodeToString([]byte(goldenWebStepOneUserData))

	t.Run("golden script", func(t *testing.T) {
		t.Parallel()
		endpoint, forms := ec2Stub(t, http.StatusOK, ec2UserDataBody(`<userData><value>`+encoded+`</value></userData>`))
		env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
		require.NoError(t, err)

		got, err := AWSInstanceUserData(context.Background(), env, NewLoopbackOnlyHTTPClient(), endpoint, testEC2InstanceID)

		require.NoError(t, err)
		assert.Equal(t, []byte(goldenWebStepOneUserData), got)
		require.Len(t, forms(), 1)
		assert.Equal(t, "DescribeInstanceAttribute", forms()[0].Get("Action"))
	})

	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{name: "missing value", status: http.StatusOK, body: ec2UserDataBody(`<userData/>`), want: "reports no user data"},
		{name: "no userData element", status: http.StatusOK, body: ec2UserDataBody(``), want: "reports no user data"},
		{name: "empty value", status: http.StatusOK, body: ec2UserDataBody(`<userData><value></value></userData>`), want: "reports empty user data"},
		{name: "not base64", status: http.StatusOK, body: ec2UserDataBody(`<userData><value>#!/bin/bash</value></userData>`), want: "not base64"},
		{name: "unauthorized", status: http.StatusForbidden, body: ec2UnauthorizedBody, want: "UnauthorizedOperation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			endpoint, _ := ec2Stub(t, tc.status, tc.body)
			env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
			require.NoError(t, err)

			got, err := AWSInstanceUserData(context.Background(), env, NewLoopbackOnlyHTTPClient(), endpoint, testEC2InstanceID)

			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), testEC2InstanceID)
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, err.Error(), testAWSSecret)
		})
	}
}

func TestDescribeAWSAMIRootAgainstAFakeEC2(t *testing.T) {
	t.Parallel()

	t.Run("8 GiB gp3 root deleted on termination", func(t *testing.T) {
		t.Parallel()
		// A non-root mapping first, so the root is found by name, not position.
		dataMapping := `<item><deviceName>/dev/sdb</deviceName><ebs><volumeSize>100</volumeSize>` +
			`<volumeType>io2</volumeType><deleteOnTermination>false</deleteOnTermination></ebs></item>`
		endpoint, forms := ec2Stub(t, http.StatusOK,
			ec2ImagesBody(ec2Image(testEC2AMIID, "ebs", "/dev/xvda", dataMapping+ec2GP3RootMapping)))
		env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
		require.NoError(t, err)

		got, err := DescribeAWSAMIRoot(context.Background(), env, NewLoopbackOnlyHTTPClient(), endpoint, testEC2AMIID)

		require.NoError(t, err)
		assert.Equal(t, AWSAMIRoot{SizeGiB: 8, VolumeType: "gp3", DeleteOnTermination: true}, got)
		require.Len(t, forms(), 1)
		assert.Equal(t, []string{testEC2AMIID}, forms()[0]["ImageId.1"])
	})

	rootImage := ec2Image(testEC2AMIID, "ebs", "/dev/xvda", ec2GP3RootMapping)
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{name: "zero images", status: http.StatusOK, body: ec2ImagesBody(), want: "returned 0 images"},
		{name: "two images", status: http.StatusOK, body: ec2ImagesBody(rootImage, rootImage), want: "returned 2 images"},
		{name: "another image", status: http.StatusOK, want: `returned image "ami-0other"`,
			body: ec2ImagesBody(ec2Image("ami-0other", "ebs", "/dev/xvda", ec2GP3RootMapping))},
		{name: "root name with no mapping", status: http.StatusOK, want: `no EBS mapping for its root device "/dev/sda1"`,
			body: ec2ImagesBody(ec2Image(testEC2AMIID, "ebs", "/dev/sda1", ec2GP3RootMapping))},
		{name: "instance-store root", status: http.StatusOK, want: `root device type is "instance-store"`,
			body: ec2ImagesBody(ec2Image(testEC2AMIID, "instance-store", "/dev/sda1",
				`<item><deviceName>/dev/sda1</deviceName><virtualName>ephemeral0</virtualName></item>`))},
		{name: "unauthorized", status: http.StatusForbidden, body: ec2UnauthorizedBody, want: "UnauthorizedOperation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			endpoint, _ := ec2Stub(t, tc.status, tc.body)
			env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
			require.NoError(t, err)

			got, err := DescribeAWSAMIRoot(context.Background(), env, NewLoopbackOnlyHTTPClient(), endpoint, testEC2AMIID)

			require.Error(t, err)
			assert.Zero(t, got)
			assert.Contains(t, err.Error(), testEC2AMIID)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestAWSEC2ReadsRefuseBeforeAnyRequest(t *testing.T) {
	t.Parallel()

	reads := map[string]func(ec2.HTTPClient, map[string]string, string, string) error{
		"AWSInstanceUserData": func(doer ec2.HTTPClient, env map[string]string, endpoint, id string) error {
			_, err := AWSInstanceUserData(context.Background(), env, doer, endpoint, id)
			return err
		},
		"DescribeAWSAMIRoot": func(doer ec2.HTTPClient, env map[string]string, endpoint, id string) error {
			_, err := DescribeAWSAMIRoot(context.Background(), env, doer, endpoint, id)
			return err
		},
	}
	validIDs := map[string]string{"AWSInstanceUserData": testEC2InstanceID, "DescribeAWSAMIRoot": testEC2AMIID}

	for name, read := range reads {
		for _, tc := range []struct {
			name, region, id, want string
			nilDoer                bool
		}{
			{name: "nil doer", region: "us-east-1", id: validIDs[name], nilDoer: true, want: "needs an HTTP client"},
			{name: "malformed region", region: "us-east-1.evil.com", id: validIDs[name], want: "not an AWS region name"},
			{name: "instance id with a path", region: "us-east-1", id: "i-../x", want: `"i-../x"`},
			{name: "ami id with a path", region: "us-east-1", id: "ami-../x", want: `"ami-../x"`},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				endpoint, forms := ec2Stub(t, http.StatusOK, "")
				env, err := AWSSealedEnv(validAWSCredFile(t), "us-east-1")
				require.NoError(t, err)
				env["AWS_REGION"] = tc.region
				var doer ec2.HTTPClient = NewLoopbackOnlyHTTPClient()
				if tc.nilDoer {
					doer = nil
				}

				err = read(doer, env, endpoint, tc.id)

				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.want)
				assert.Empty(t, forms())
			})
		}
	}
}

func TestAWSStateInstanceID(t *testing.T) {
	t.Parallel()
	instance := func(id string) map[string]any {
		return awsStateResource("managed", "aws_instance", map[string]any{"id": id})
	}
	vpc := awsStateResource("managed", "aws_vpc", map[string]any{"id": "vpc-1"})

	t.Run("one instance", func(t *testing.T) {
		t.Parallel()
		id, err := AWSStateInstanceID(awsStateWorkDir(t, vpc, instance(testEC2InstanceID)))
		require.NoError(t, err)
		assert.Equal(t, testEC2InstanceID, id)
	})

	twoInstancesOneResource := instance("i-1")
	twoInstancesOneResource["instances"] = []any{
		map[string]any{"attributes": map[string]any{"id": "i-1"}},
		map[string]any{"attributes": map[string]any{"id": "i-2"}},
	}
	for name, tc := range map[string]struct {
		workDir func(t *testing.T) string
		want    string
	}{
		"zero instances": {
			workDir: func(t *testing.T) string { return awsStateWorkDir(t, vpc) },
			want:    "holds 0 managed aws_instance",
		},
		"two instance resources": {
			workDir: func(t *testing.T) string { return awsStateWorkDir(t, instance("i-1"), instance("i-2")) },
			want:    "holds 2 managed aws_instance",
		},
		"two instances of one resource": {
			workDir: func(t *testing.T) string { return awsStateWorkDir(t, twoInstancesOneResource) },
			want:    "holds 2 managed aws_instance",
		},
		"only a data source": {
			workDir: func(t *testing.T) string {
				return awsStateWorkDir(t, awsStateResource("data", "aws_instance", map[string]any{"id": testEC2InstanceID}))
			},
			want: "holds 0 managed aws_instance",
		},
		"an instance with no id": {
			workDir: func(t *testing.T) string { return awsStateWorkDir(t, instance("")) },
			want:    "has no id",
		},
		"no state": {
			workDir: func(t *testing.T) string { return t.TempDir() },
			want:    "read live terraform state",
		},
		"a corrupt state": {
			workDir: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, LiveStateFilename), []byte("{"), 0o600))
				return dir
			},
			want: "decode live terraform state",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			id, err := AWSStateInstanceID(tc.workDir(t))
			require.Error(t, err)
			assert.Empty(t, id)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
