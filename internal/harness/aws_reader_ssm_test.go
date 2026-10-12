package harness

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readSSMFake reads readSet with ssm answering each GetParameter in turn
// with answers, and ec2 recording every request.
func readSSMFake(t *testing.T, ec2 *ec2RecordingFake, answers []awsCannedAnswer, readSet ...AWSReadEntry) (AWSRecordedRead, error) {
	t.Helper()
	doers := newAWSReadTestDoers()
	doers.ssm.queue = answers
	doer := doers.doer()
	doer[awsReadEC2Host] = ec2
	return ReadAWSAccount(context.Background(), awsReadTestInput(doer, readSet...))
}

func ssmParameterEntry(name string) AWSReadEntry {
	return readEntry("ssm", "json", "GetParameter", `{"Name":"`+name+`"}`)
}

func ssmParameterOK(name, value string) awsCannedAnswer {
	return awsOK(`{"Parameter":{"Name":"` + name + `","Type":"String","Value":"` + value + `","Version":1}}`)
}

// ec2ImageAnswer answers each DescribeImages with the image it asks for.
func ec2ImageAnswer(form url.Values) (awsCannedAnswer, bool) {
	return awsOK(`<DescribeImagesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><imagesSet><item><imageId>` +
		form.Get("ImageId.1") + `</imageId><imageOwnerId>137112412989</imageOwnerId></item></imagesSet></DescribeImagesResponse>`), true
}

func TestReadAWSAccountReadsTheImageAPublicParameterNames(t *testing.T) {
	t.Parallel()

	const (
		public  = AWSAL2023AMIParameter
		private = "/app/ami"
		other   = "/aws/service/global-infrastructure/regions/eu-west-1/longName"
		absent  = "/aws/service/absent"
		again   = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-6.1-x86_64"
	)
	notFound := awsCannedAnswer{status: http.StatusBadRequest, body: `{"__type":"ParameterNotFound"}`}
	ec2 := &ec2RecordingFake{answer: ec2ImageAnswer}

	read, err := readSSMFake(t, ec2,
		[]awsCannedAnswer{
			ssmParameterOK(public, "ami-0abc"),
			ssmParameterOK(private, "ami-0def"),
			ssmParameterOK(other, "Europe (Ireland)"),
			notFound,
			ssmParameterOK(again, "ami-0abc"),
		},
		ssmParameterEntry(public), ssmParameterEntry(private), ssmParameterEntry(other), ssmParameterEntry(absent), ssmParameterEntry(again),
	)
	require.NoError(t, err)

	want := ec2Form("DescribeImages", map[string]string{"ImageId.1": "ami-0abc"})
	assert.Equal(t, []url.Values{want}, ec2.forms, "one DescribeImages, for the public AMI parameter's value only")
	require.Len(t, read.Exchanges, 6)
	assert.Equal(t, "DescribeImages", read.Exchanges[5].Action, "the image is recorded after the replay")
	assert.JSONEq(t, `{"ImageId.1":"ami-0abc"}`, string(read.Exchanges[5].Params))
}

func TestReadAWSAccountSendsNoDescribeImagesWithoutAPublicAMIParameter(t *testing.T) {
	t.Parallel()

	ec2 := &ec2RecordingFake{answer: func(url.Values) (awsCannedAnswer, bool) { return awsCannedAnswer{}, false }}

	_, err := readSSMFake(t, ec2, []awsCannedAnswer{ssmParameterOK("/app/ami", "ami-0def")}, ssmParameterEntry("/app/ami"))

	require.NoError(t, err)
	assert.Empty(t, ec2.forms)
}

func TestReadAWSAccountFailsAGetParameterThatNamesNoParameter(t *testing.T) {
	t.Parallel()

	ec2 := &ec2RecordingFake{answer: func(url.Values) (awsCannedAnswer, bool) { return awsCannedAnswer{}, false }}

	read, err := readSSMFake(t, ec2, []awsCannedAnswer{awsOK(`{}`)}, ssmParameterEntry(AWSAL2023AMIParameter))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "the answer names no parameter")
	assert.Empty(t, read.Exchanges)
	assert.Empty(t, ec2.forms)
}

func TestReadAWSAccountFailsAnImageTheReadDoesNotHold(t *testing.T) {
	t.Parallel()

	for name, answer := range map[string]awsCannedAnswer{
		"an empty imagesSet":    awsOK(`<DescribeImagesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><imagesSet/></DescribeImagesResponse>`),
		"InvalidAMIID.NotFound": {status: http.StatusBadRequest, body: awsQueryErrorBody("http://ec2.amazonaws.com/doc/2016-11-15/", "InvalidAMIID.NotFound")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ec2 := &ec2RecordingFake{answer: func(url.Values) (awsCannedAnswer, bool) { return answer, true }}

			read, err := readSSMFake(t, ec2, []awsCannedAnswer{ssmParameterOK(AWSAL2023AMIParameter, "ami-0abc")}, ssmParameterEntry(AWSAL2023AMIParameter))

			require.Error(t, err)
			assert.Contains(t, err.Error(), `image "ami-0abc" of parameter "`+AWSAL2023AMIParameter+`"`)
			assert.Contains(t, err.Error(), "DescribeImages answered without the image")
			assert.Empty(t, read.Exchanges)
			assert.Len(t, ec2.forms, 1)
		})
	}
}
