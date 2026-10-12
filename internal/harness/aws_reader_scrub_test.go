package harness

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scrub fixtures: a recorded read of real wire shapes and a v4
// tfstate, both holding scrubTestAccount, the read's account.
const (
	scrubTestRead    = "testdata/aws_reader_scrub/read.json"
	scrubTestState   = "testdata/aws_reader_scrub/state.json"
	scrubTestAccount = "246813579024"
	// scrubTestStray is a 12-digit number that is not the read's account,
	// so the byte map leaves it.
	scrubTestStray = "999988887777"
)

// scrubFixtures writes the fixtures, each through its edit, into a temp
// dir and returns their paths and an out dir that does not exist yet.
func scrubFixtures(t *testing.T, editRead, editState func(string) string) (readPath, statePath, outDir string) {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []struct {
		from, to string
		edit     func(string) string
	}{{scrubTestRead, "read.json", editRead}, {scrubTestState, "state.json", editState}} {
		data, err := os.ReadFile(f.from)
		require.NoError(t, err)
		if f.edit != nil {
			edited := f.edit(string(data))
			require.NotEqual(t, string(data), edited, "the edit must change the fixture")
			data = []byte(edited)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.to), data, 0o600))
	}
	return filepath.Join(dir, "read.json"), filepath.Join(dir, "state.json"), filepath.Join(dir, "out")
}

// scrubbedFixtures scrubs the fixtures and returns the two files written.
func scrubbedFixtures(t *testing.T) (read, state []byte) {
	t.Helper()
	readPath, statePath, outDir := scrubFixtures(t, nil, nil)
	require.NoError(t, WriteScrubbedAWSRead(readPath, statePath, outDir))
	read, err := os.ReadFile(filepath.Join(outDir, AWSScrubbedReadFile))
	require.NoError(t, err)
	state, err = os.ReadFile(filepath.Join(outDir, AWSScrubbedStateFile))
	require.NoError(t, err)
	return read, state
}

func TestWriteScrubbedAWSReadMapsTheAccountAndPlacesThePlaceholders(t *testing.T) {
	fixtureRead, err := os.ReadFile(scrubTestRead)
	require.NoError(t, err)
	// A RequestId whose last group is twelve decimal digits: it reads as
	// an account until it is replaced.
	require.Contains(t, string(fixtureRead), "<requestId>59dbff89-35bd-4eac-99ed-504863521975</requestId>")

	read, state := scrubbedFixtures(t)
	var decoded AWSRecordedRead
	require.NoError(t, decodeOneJSON(read, &decoded))
	bodies := ""
	for _, ex := range decoded.Exchanges {
		bodies += ex.Body
	}

	for name, file := range map[string][]byte{"read": read, "state": state} {
		for _, planted := range []string{
			scrubTestAccount, "AIDATPLANTEDUSERID001", "IyEvYmluL2Jhc2gKZWNobyBwbGFudGVkLXNlY3JldAo=",
			"planted-db-password", "504863521975", "6f0c8f7e-3b1a-4d2c-9e5f-a1b2c3d4e5f6",
			"e3cf5b39bc793e8151d1893fb3992ebdad89c0d2", "infrafactory-reader", "planted-password-data",
			"planted-output-secret", "infrafactory-layer3", "AIDATPLANTEDLAYER3001",
		} {
			assert.NotContains(t, string(file), planted, "the scrubbed %s", name)
		}
	}
	assert.Equal(t, AWSScrubAccount, decoded.Account)
	assert.Contains(t, string(read), `"Filter.1.Value.1": "`+AWSScrubAccount+`"`)
	for _, want := range []string{
		"<Arn>arn:aws:iam::" + AWSScrubAccount + ":user/" + AWSScrubPrincipal + "</Arn>",
		"<Account>" + AWSScrubAccount + "</Account>",
		"<UserId>" + AWSScrubUserID + "</UserId>",
		"<ownerId>" + AWSScrubAccount + "</ownerId>",
		"<securityGroupArn>arn:aws:ec2:eu-west-1:" + AWSScrubAccount + ":security-group/sg-0f1e2d3c4b5a69788</securityGroupArn>",
		"%3Aaws%3Assm%3Aeu-west-1%3A" + AWSScrubAccount + "%3Aparameter%2Fapp%2F%2A",
		"<userData>\n        <value>" + AWSScrubUserData + "</value>\n    </userData>",
		`"Value":"` + AWSScrubParameterValue + `"`,
		`"ARN":"arn:aws:ssm:eu-west-1:` + AWSScrubAccount + `:parameter/app/db/password"`,
		// Kept: a tag's value, a public parameter's value and SSM's
		// LastModifiedDate, whose fraction is twelve digits.
		"<key>Name</key>\n                    <value>web</value>",
		`"Value":"ami-0c1d2e3f4a5b6c7d8"`,
		`"LastModifiedDate":1.760175002193E9`,
		"<requestId>" + AWSScrubRequestID + "</requestId>",
		"<RequestId>" + AWSScrubRequestID + "</RequestId>",
		"<RequestID>" + AWSScrubRequestID + "</RequestID>",
	} {
		assert.Contains(t, bodies, want)
	}
	assert.Equal(t, 5, strings.Count(bodies, AWSScrubRequestID), "every RequestId")
	for _, want := range []string{
		`"arn": "arn:aws:ec2:eu-west-1:` + AWSScrubAccount + `:instance/i-0123abcd4567ef890"`,
		`"owner_id": "` + AWSScrubAccount + `"`,
		`"value": "arn:aws:ec2:eu-west-1:` + AWSScrubAccount + `:security-group/sg-0f1e2d3c4b5a69788"`,
		`"lineage": "` + AWSScrubLineage + `"`,
		`"user_data": "` + AWSScrubUserData + `"`,
		`"password_data": "` + AWSScrubSensitiveValue + `"`,
		`"arn": "arn:aws:iam::` + AWSScrubAccount + `:user/` + AWSScrubPrincipal + `"`,
		`"user_id": "` + AWSScrubUserID + `"`,
		// The provider marks a public parameter's value sensitive; it stays.
		`"value": "ami-0c1d2e3f4a5b6c7d8"`,
	} {
		assert.Contains(t, string(state), want)
	}
}

// A volume's KMS key ARN names the read's account; the scrub maps it,
// as it maps the account everywhere.
func TestWriteScrubbedAWSReadMapsTheAccountInAVolumesKMSKeyARN(t *testing.T) {
	const kmsKey = "arn:aws:kms:eu-west-1:" + scrubTestAccount + ":key/1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	const volumes = `{
      "service": "ec2",
      "protocol": "query",
      "action": "DescribeVolumes",
      "params": {"Filter.1.Name": "attachment.instance-id", "Filter.1.Value.1": "i-0123abcd4567ef890"},
      "status": 200,
      "body": "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<DescribeVolumesResponse xmlns=\"http://ec2.amazonaws.com/doc/2016-11-15/\"><volumeSet><item><volumeId>vol-0123abcd4567ef890</volumeId><size>8</size><availabilityZone>eu-west-1a</availabilityZone><status>in-use</status><attachmentSet><item><volumeId>vol-0123abcd4567ef890</volumeId><instanceId>i-0123abcd4567ef890</instanceId><device>/dev/xvda</device><status>attached</status><deleteOnTermination>true</deleteOnTermination></item></attachmentSet><encrypted>true</encrypted><kmsKeyId>` + kmsKey + `</kmsKeyId><volumeType>gp3</volumeType></item></volumeSet></DescribeVolumesResponse>"
    },`
	readPath, statePath, outDir := scrubFixtures(t, func(s string) string {
		return strings.Replace(s, `"exchanges": [`, `"exchanges": [`+volumes, 1)
	}, nil)
	require.NoError(t, WriteScrubbedAWSRead(readPath, statePath, outDir))
	read, err := os.ReadFile(filepath.Join(outDir, AWSScrubbedReadFile))
	require.NoError(t, err)

	assert.NotContains(t, string(read), scrubTestAccount)
	assert.Contains(t, string(read), "<kmsKeyId>arn:aws:kms:eu-west-1:"+AWSScrubAccount+":key/1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d</kmsKeyId>")
}

// Every key of the input state survives, and only the account, the
// principal's name and user id, the lineage, the user data, the sensitive
// attributes and the sensitive output differ.
func TestWriteScrubbedAWSReadKeepsEveryKeyOfTheState(t *testing.T) {
	_, state := scrubbedFixtures(t)
	fixture, err := os.ReadFile(scrubTestState)
	require.NoError(t, err)

	var want, got map[string]any
	require.NoError(t, decodeOneJSON(bytes.ReplaceAll(fixture, []byte(scrubTestAccount), []byte(AWSScrubAccount)), &want))
	require.NoError(t, decodeOneJSON(state, &got))
	want["lineage"] = AWSScrubLineage
	instance := want["resources"].([]any)[1].(map[string]any)["instances"].([]any)[0].(map[string]any)
	instance["attributes"].(map[string]any)["user_data"] = AWSScrubUserData
	instance["attributes"].(map[string]any)["password_data"] = AWSScrubSensitiveValue
	want["outputs"].(map[string]any)["db_password"].(map[string]any)["value"] = AWSScrubSensitiveValue
	caller := want["resources"].([]any)[4].(map[string]any)["instances"].([]any)[0].(map[string]any)["attributes"].(map[string]any)
	caller["arn"] = "arn:aws:iam::" + AWSScrubAccount + ":user/" + AWSScrubPrincipal
	caller["user_id"] = AWSScrubUserID
	assert.Equal(t, want, got)
	for _, key := range []string{"schema_version", "private", "dependencies", "sensitive_attributes"} {
		assert.Contains(t, instance, key)
	}
}

func TestWriteScrubbedAWSReadRefusesALeakAndWritesNothing(t *testing.T) {
	replace := func(old, new string) func(string) string {
		return func(s string) string { return strings.Replace(s, old, new, 1) }
	}
	tagValue := "<value>web</value>"
	cases := []struct {
		name                string
		editRead, editState func(string) string
		planted, wantPath   string
	}{
		{
			name:     "a stray 12-digit number in a body",
			editRead: replace(tagValue, "<value>"+scrubTestStray+"</value>"),
			planted:  scrubTestStray, wantPath: "$.exchanges[1].body: holds a 12-digit number",
		},
		{
			// \u-escaped, so the byte map cannot see it; the check decodes.
			name:     "the account in params",
			editRead: replace(`"Filter.1.Value.1": "`+scrubTestAccount+`"`, `"Filter.1.Value.1": "`+jsonUnicodeEscape(scrubTestAccount)+`"`),
			planted:  scrubTestAccount, wantPath: "$.exchanges[1].params.Filter.1.Value.1: holds a 12-digit number",
		},
		{
			// The scrub replaces userData in DescribeInstanceAttribute only;
			// the check reads every body.
			name:     "an unreplaced userData",
			editRead: replace(`"action": "DescribeInstanceAttribute"`, `"action": "DescribeInstances"`),
			planted:  "IyEvYmluL2Jhc2gK", wantPath: "$.exchanges[2].body: holds a userData that is not the placeholder",
		},
		{
			// The scrub replaces Parameter.Value only; the check reads every
			// Value.
			name:     "an unreplaced private Parameter.Value",
			editRead: replace(`\"Parameter\":`, `\"Param\":`),
			planted:  "planted-db-password", wantPath: "$.exchanges[3].body: $.Param.Value: is neither the placeholder",
		},
		{
			name:     "an AKIA-style key id",
			editRead: replace(tagValue, "<value>AKIAPLANTEDKEYID00001</value>"),
			planted:  "AKIAPLANTEDKEYID00001", wantPath: "$.exchanges[1].body: holds an AWS unique or key id",
		},
		{
			name:     "an AIDA-style unique id",
			editRead: replace("<groupDescription>web ingress</groupDescription>", "<groupDescription>AIDAPLANTEDUSERID0002</groupDescription>"),
			planted:  "AIDAPLANTEDUSERID0002", wantPath: "$.exchanges[1].body: holds an AWS unique or key id",
		},
		{
			// %22 glues the digits onto the quote: only the decoded view
			// shows a 12-digit run.
			name:     "a URL-encoded stray account",
			editRead: replace("%22Effect%22", "%22"+scrubTestStray+"%22"),
			planted:  scrubTestStray, wantPath: "$.exchanges[5].body: holds a 12-digit number",
		},
		{
			name:      "a leak in the state",
			editState: replace(`"description": "web ingress"`, `"description": "`+scrubTestStray+`"`),
			planted:   scrubTestStray, wantPath: "$.resources[2].instances[0].attributes.description: holds a 12-digit number",
		},
		{
			name:      "a private SSM parameter in the state",
			editState: replace(`"name": "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"`, `"name": "/app/ami"`),
			// The scrub hides value, which the provider marks sensitive;
			// insecure_value is not marked, so the check refuses it.
			planted: "ami-0c1d2e3f4a5b6c7d8", wantPath: "$.resources[0].instances[0].attributes.insecure_value: is a private SSM parameter's value",
		},
		{
			// A principal with no name the scrub knows how to replace.
			name:     "a root principal",
			editRead: replace(":user/infrafactory-reader</Arn>", ":root</Arn>"),
			wantPath: "$.exchanges[0].body: holds a GetCallerIdentity Arn whose name is not the placeholder",
		},
		{
			name: "a sensitive path the scrub cannot follow",
			editState: replace(`"type": "get_attr",
                "value": "password_data"`, `"type": "index",
                "value": "password_data"`),
			planted: "planted-password-data", wantPath: "$.resources[1].instances[0].sensitive_attributes: holds a path that does not start at an attribute",
		},
		{
			// URL-encoded, so the scrub cannot see the name; the check
			// decodes it.
			name:      "a URL-encoded principal in the state",
			editState: replace(`"description": "web ingress"`, `"description": "arn%3Aaws%3Aiam%3A%3A`+scrubTestAccount+`%3Auser%2Falice"`),
			planted:   "alice", wantPath: "$.resources[2].instances[0].attributes.description: names an IAM user or assumed role",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readPath, statePath, outDir := scrubFixtures(t, tc.editRead, tc.editState)
			err := WriteScrubbedAWSRead(readPath, statePath, outDir)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantPath)
			if tc.planted != "" {
				assert.NotContains(t, err.Error(), tc.planted)
			}
			assert.NotContains(t, err.Error(), scrubTestAccount)
			assert.NoDirExists(t, outDir, "the tool writes nothing")
		})
	}
}

// jsonUnicodeEscape writes every byte of s as a JSON \u escape.
func jsonUnicodeEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

// The check also guards committed fixtures, which no scrub wrote: these
// edit a scrubbed read directly.
func TestCheckScrubbedRead(t *testing.T) {
	read, state := scrubbedFixtures(t)
	require.NoError(t, CheckScrubbedRead("read", read, "state", state))

	groupName := "<groupName>web</groupName>"
	cases := []struct {
		name, old, new string
		inState        bool
		// planted is what the error must not quote.
		wantErr, planted string
	}{
		{name: "a 13-digit run is not a hit", old: groupName, new: "<groupName>1234567890123</groupName>"},
		{name: "an 11-digit run is not a hit", old: groupName, new: "<groupName>12345678901</groupName>"},
		{
			name: "a repeated key", old: `"region": "eu-west-1",`, new: `"region": "eu-west-1", "region": "eu-west-1",`,
			wantErr: "$.region: repeats a key",
		},
		{
			name: "a key repeated in another case", old: `"body": "<GetCallerIdentityResponse`, new: `"Body": "x", "body": "<GetCallerIdentityResponse`,
			wantErr: "$.exchanges[0].body: repeats a key",
		},
		{
			name: "a Value in another case", old: `\"Value\":\"` + AWSScrubParameterValue + `\"`, new: `\"value\":\"planted-db-password\"`,
			wantErr: "$.exchanges[3].body: $.Parameter.value: is neither the placeholder", planted: "planted-db-password",
		},
		{
			name: "a run split by CDATA", old: groupName, new: "<groupName>2468<![CDATA[13579024]]></groupName>",
			wantErr: "$.exchanges[1].body: holds a 12-digit number", planted: "13579024",
		},
		{
			name: "a run spelled with character references", old: groupName, new: "<groupName>&#50;46813579024</groupName>",
			wantErr: "$.exchanges[1].body: holds a 12-digit number", planted: "46813579024",
		},
		{
			name: "userData text outside a value", old: "<value>" + AWSScrubUserData + "</value>", new: "c2VjcmV0",
			wantErr: "$.exchanges[2].body: holds a userData that is not the placeholder", planted: "c2VjcmV0",
		},
		{
			name: "an account that is not the synthetic id", old: `"account": "` + AWSScrubAccount + `"`, new: `"account": "1234"`,
			wantErr: "$.account: is not the synthetic account id", planted: "1234",
		},
		{
			name: "a number glued to an exponent fraction", old: groupName, new: "<groupName>12.123456789012E9</groupName>",
			wantErr: "$.exchanges[1].body: holds a 12-digit number", planted: "123456789012",
		},
		{
			// Base64 user data passes the text rules; a comment is refused
			// whole.
			name: "user data in a comment", old: "<value>" + AWSScrubUserData + "</value>", new: "<!-- IyEvYmluL2Jhc2gK -->",
			wantErr: "$.exchanges[2].body: holds an XML comment or directive", planted: "IyEvYmluL2Jhc2gK",
		},
		{
			name: "a DOCTYPE", old: "<DescribeSecurityGroupsResponse ", new: "<!DOCTYPE x><DescribeSecurityGroupsResponse ",
			wantErr: "$.exchanges[1].body: holds an XML comment or directive",
		},
		{
			name: "a processing instruction", old: "<DescribeSecurityGroupsResponse ", new: "<?app c2VjcmV0?><DescribeSecurityGroupsResponse ",
			wantErr: "$.exchanges[1].body: holds a processing instruction other than the XML declaration", planted: "c2VjcmV0",
		},
		{
			name: "a principal name in GetCallerIdentity's Arn", old: ":user/" + AWSScrubPrincipal, new: ":user/alice",
			wantErr: "$.exchanges[0].body: names an IAM user or assumed role other than the placeholder", planted: "alice",
		},
		{
			name: "a principal name in the state", inState: true, old: ":user/" + AWSScrubPrincipal, new: ":user/infrafactory-layer3",
			wantErr: "$.resources[4].instances[0].attributes.arn: names an IAM user or assumed role", planted: "infrafactory-layer3",
		},
		{
			name: "a sensitive output in the state", inState: true,
			old: `"value": "` + AWSScrubSensitiveValue + `"`, new: `"value": "hunter2-secret"`,
			wantErr: "$.outputs.db_password.value: is a sensitive output's value", planted: "hunter2-secret",
		},
		{
			// The exponent exemption is for JSON numbers only.
			name: "a 12-digit fraction in a string", old: "<groupDescription>web ingress</groupDescription>",
			new:     "<groupDescription>v1.987654321098e</groupDescription>",
			wantErr: "$.exchanges[1].body: holds a 12-digit number", planted: "987654321098",
		},
		{
			name: "a 12-digit JSON number", old: `\"Version\":3`, new: `\"Version\":999988887777`,
			wantErr: "$.exchanges[3].body: $.Parameter.Version: holds a 12-digit number", planted: "999988887777",
		},
		{
			// Seen raw only: the decoded XML check reads text, not names.
			name: "a 12-digit element name", old: "<groupName>web</groupName>", new: "<groupName>web</groupName><x999988887777/>",
			wantErr: "$.exchanges[1].body: holds a 12-digit number", planted: "999988887777",
		},
		{
			// A public parameter's value passes; its other sensitive
			// attributes do not.
			name: "another sensitive attribute of a public parameter", inState: true,
			old: `"value": "value"`, new: `"value": "name"`,
			wantErr: "$.resources[0].instances[0].attributes.name: is listed in sensitive_attributes", planted: "al2023",
		},
		{
			name: "a sensitive attribute in the state", inState: true,
			old: `"password_data": "` + AWSScrubSensitiveValue + `"`, new: `"password_data": "planted-password-data"`,
			wantErr: "$.resources[1].instances[0].attributes.password_data: is listed in sensitive_attributes", planted: "planted-password-data",
		},
		{
			name: "an unreplaced user_data in the state", inState: true,
			old: `"user_data": "` + AWSScrubUserData + `"`, new: `"user_data": "e3cf5b39bc793e8151d1893fb3992ebdad89c0d2"`,
			wantErr: "$.resources[1].instances[0].attributes.user_data: is neither null nor the placeholder", planted: "e3cf5b39",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{"read": read, "state": state}
			name := map[bool]string{false: "read", true: "state"}[tc.inState]
			edited := strings.Replace(string(files[name]), tc.old, tc.new, 1)
			require.NotEqual(t, string(files[name]), edited)
			files[name] = []byte(edited)
			err := CheckScrubbedRead("read", files["read"], "state", files["state"])
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, "scrub check: "+name+": "+tc.wantErr)
			if tc.planted != "" {
				assert.NotContains(t, err.Error(), tc.planted)
			}
		})
	}
}

func TestWriteScrubbedAWSReadHidesAPublicParametersOtherSensitiveAttributes(t *testing.T) {
	readPath, statePath, outDir := scrubFixtures(t, nil, func(s string) string {
		return strings.Replace(s, `"value": "value"`, `"value": "id"`, 1)
	})
	require.NoError(t, WriteScrubbedAWSRead(readPath, statePath, outDir))
	data, err := os.ReadFile(filepath.Join(outDir, AWSScrubbedStateFile))
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, decodeOneJSON(data, &state))
	attributes := state["resources"].([]any)[0].(map[string]any)["instances"].([]any)[0].(map[string]any)["attributes"].(map[string]any)
	assert.Equal(t, AWSScrubSensitiveValue, attributes["id"])
	assert.Equal(t, "ami-0c1d2e3f4a5b6c7d8", attributes["value"])
}

func TestWriteScrubbedAWSReadNamesNoAssumedRole(t *testing.T) {
	readPath, statePath, outDir := scrubFixtures(t, func(s string) string {
		return strings.Replace(s, "arn:aws:iam::"+scrubTestAccount+":user/infrafactory-reader",
			"arn:aws:sts::"+scrubTestAccount+":assumed-role/AWSReservedSSO_Admin_x/alice@corp.com", 1)
	}, nil)
	require.NoError(t, WriteScrubbedAWSRead(readPath, statePath, outDir))
	read, err := os.ReadFile(filepath.Join(outDir, AWSScrubbedReadFile))
	require.NoError(t, err)
	assert.Contains(t, string(read), "<Arn>arn:aws:sts::"+AWSScrubAccount+":assumed-role/"+AWSScrubPrincipal+"</Arn>")
	assert.NotContains(t, string(read), "AWSReservedSSO")
	assert.NotContains(t, string(read), "alice")
}

// Every placeholder and the synthetic id pass the check that refuses ids.
func TestScrubPlaceholdersPassTheCheck(t *testing.T) {
	for _, placeholder := range []string{
		AWSScrubAccount, AWSScrubUserData, AWSScrubParameterValue, AWSScrubUserID, AWSScrubRequestID, AWSScrubLineage,
		AWSScrubPrincipal, AWSScrubSensitiveValue,
	} {
		assert.NoError(t, checkScrubbedText(placeholder), placeholder)
	}
}

func TestWriteScrubbedAWSReadWritesMode0600(t *testing.T) {
	readPath, statePath, outDir := scrubFixtures(t, nil, nil)
	require.NoError(t, os.MkdirAll(outDir, 0o755))
	// A file already there with a wider mode is replaced, not written into.
	require.NoError(t, os.WriteFile(filepath.Join(outDir, AWSScrubbedReadFile), []byte("{}"), 0o644))

	require.NoError(t, WriteScrubbedAWSRead(readPath, statePath, outDir))
	for _, name := range []string{AWSScrubbedReadFile, AWSScrubbedStateFile} {
		info, err := os.Stat(filepath.Join(outDir, name))
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm(), name)
	}
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "no temp file is left")
}

func TestScrubAWSReadRefusesAnAccountThatIsNotTwelveDigits(t *testing.T) {
	readPath, statePath, outDir := scrubFixtures(t, func(s string) string {
		return strings.Replace(s, `"account": "`+scrubTestAccount+`"`, `"account": ""`, 1)
	}, nil)
	err := WriteScrubbedAWSRead(readPath, statePath, outDir)
	assert.ErrorContains(t, err, "its account is not a 12-digit id")
	assert.NoDirExists(t, outDir)
}
