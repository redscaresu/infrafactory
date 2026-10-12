package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// The key planted in the read's credential file; no output may hold any
// of it.
const (
	readKeyID  = "AKIAREHEARSEREADKEY1"
	readSecret = "rehearse-read-secret-access-key-value"
	readToken  = "rehearse-read-session-token-value"
)

const (
	readAccount  = "123456789012"
	readRegion   = "eu-west-1"
	readSetFile  = "read-set.json"
	readKeyFile  = "reader.env"
	readOutDir   = ".infrafactory/reads/step0"
	readVPCEntry = `{"service": "ec2", "protocol": "query", "action": "DescribeVpcs",` +
		` "params": {"Filter.1.Name": "tag:Name", "Filter.1.Value.1": "web"}}`
)

// readDoer answers the read as AWS would a key of account: STS who it
// is, EC2 the preflight's DryRun requests with dryRunCode
// (UnauthorizedOperation when empty), and DescribeVpcs with no VPCs,
// calling onDescribe first when set. It records each request's action.
type readDoer struct {
	account, dryRunCode string
	onDescribe          func()
	actions             []string
}

func (d *readDoer) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	action := form.Get("Action")
	d.actions = append(d.actions, action)
	status, answer := http.StatusOK, ""
	switch {
	case action == "GetCallerIdentity":
		answer = "<GetCallerIdentityResponse><GetCallerIdentityResult><Arn>arn:aws:iam::" + d.account +
			":user/infrafactory-reader</Arn><Account>" + d.account + "</Account></GetCallerIdentityResult></GetCallerIdentityResponse>"
	case form.Get("DryRun") == "true" && d.dryRunCode == "":
		status, answer = http.StatusForbidden, ec2ErrorAnswer("UnauthorizedOperation")
	case form.Get("DryRun") == "true":
		status, answer = http.StatusPreconditionFailed, ec2ErrorAnswer(d.dryRunCode)
	case action == "DescribeVpcs":
		if d.onDescribe != nil {
			d.onDescribe()
		}
		answer = "<DescribeVpcsResponse><vpcSet/></DescribeVpcsResponse>"
	default:
		status, answer = http.StatusBadRequest, ec2ErrorAnswer("InvalidAction")
	}
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": {"text/xml"}},
		Body: io.NopCloser(strings.NewReader(answer)), Request: req,
	}, nil
}

func ec2ErrorAnswer(code string) string {
	return "<Response><Errors><Error><Code>" + code + "</Code><Message>m</Message></Error></Errors></Response>"
}

// readRepo is a repo under a temp dir holding the key file and a read set
// of entries, and the temp dir.
func readRepo(t *testing.T, entries ...string) (tmp, repo string) {
	t.Helper()
	// Resolved, so the only symlink a path meets is one a test plants.
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo = filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	writeReadInputs(t, repo, entries...)
	return tmp, repo
}

// writeReadInputs writes the key file and a read set of entries into dir.
func writeReadInputs(t *testing.T, dir string, entries ...string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(dir, readSetFile), "["+strings.Join(entries, ",")+"]\n")
	key := "AWS_ACCESS_KEY_ID=" + readKeyID + "\nAWS_SECRET_ACCESS_KEY=" + readSecret + "\nAWS_SESSION_TOKEN=" + readToken + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, readKeyFile), []byte(key), 0o600))
}

// readFlags are the flags but --out, without the ones named in drop.
func readFlags(drop ...string) []string {
	var args []string
	for _, kv := range [][2]string{
		{"--account", readAccount}, {"--region", readRegion},
		{"--credentials", readKeyFile}, {"--read-set", readSetFile},
	} {
		if !slices.Contains(drop, kv[0]) {
			args = append(args, kv[0], kv[1])
		}
	}
	return args
}

// readRoot is the root command with read built from r.
func readRoot(r rehearseRead) *cobra.Command {
	root := NewRootCmd()
	group, _, _ := root.Find([]string{"rehearse"})
	read, _, _ := group.Find([]string{"read"})
	group.RemoveCommand(read)
	group.AddCommand(newRehearseReadCmd(r))
	return root
}

type readRun struct {
	stdout, stderr string
	err            error
}

// runRead runs `rehearse read args` from wd through doer, and holds every
// output to never carrying the key.
func runRead(t *testing.T, wd string, doer *readDoer, args ...string) readRun {
	t.Helper()
	root := readRoot(rehearseRead{doer: doer, wd: wd})
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(append([]string{"rehearse", "read"}, args...))
	run := readRun{err: root.Execute(), stdout: stdout.String(), stderr: stderr.String()}
	outputs := map[string]string{"stdout": run.stdout, "stderr": run.stderr}
	if run.err != nil {
		outputs["error"] = run.err.Error()
	}
	for name, out := range outputs {
		for _, value := range []string{readKeyID, readSecret, readToken} {
			assert.NotContains(t, out, value, "%s holds a key value", name)
		}
	}
	return run
}

func TestRehearseReadRefusesAnOutOutsideInfrafactory(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		out  func(tmp string) string
	}{
		{"a repo path", func(string) string { return "reads/step0" }},
		{"a path outside the repo", func(tmp string) string { return filepath.Join(tmp, "elsewhere") }},
		{".infrafactory/../x", func(string) string { return ".infrafactory/../x" }},
		{".infrafactory itself", func(string) string { return ".infrafactory" }},
		{"a symlink inside .infrafactory that points out", func(string) string { return ".infrafactory/link/step0" }},
		{".infrafactory/runs/x", func(string) string { return ".infrafactory/runs/x" }},
		{".infrafactory/live/x", func(string) string { return ".infrafactory/live/x" }},
		{".infrafactory/RUNS/x, the run store on a case-insensitive file system", func(string) string { return ".infrafactory/RUNS/x" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmp, repo := readRepo(t, readVPCEntry)
			outside := filepath.Join(tmp, "outside")
			require.NoError(t, os.MkdirAll(outside, 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(repo, ".infrafactory"), 0o700))
			require.NoError(t, os.Symlink(outside, filepath.Join(repo, ".infrafactory", "link")))
			doer := &readDoer{account: readAccount}

			run := runRead(t, repo, doer, append(readFlags(), "--out", tc.out(tmp))...)

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), "--out")
			assert.Empty(t, doer.actions, "no request")
			assert.NoDirExists(t, filepath.Join(outside, "step0"))
			assert.NoDirExists(t, filepath.Join(repo, "reads"))
			assert.NoDirExists(t, filepath.Join(repo, "x"))
		})
	}
}

func TestRehearseReadWritesTheRecordedReadPrivately(t *testing.T) {
	t.Parallel()
	_, repo := readRepo(t, readVPCEntry)
	doer := &readDoer{account: readAccount}

	run := runRead(t, repo, doer, append(readFlags(), "--principal", "arn:aws:iam::"+readAccount+":user/infrafactory-reader", "--out", readOutDir)...)

	require.NoError(t, run.err)
	out, err := filepath.EvalSymlinks(filepath.Join(repo, readOutDir))
	require.NoError(t, err)
	assert.Contains(t, run.stdout, "Recorded read: "+filepath.Join(out, rehearseReadFile))
	dir, err := os.Stat(out)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
	file, err := os.Stat(filepath.Join(out, rehearseReadFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), file.Mode().Perm())
	reads, err := os.Stat(filepath.Dir(out))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), reads.Mode().Perm(), "the level between .infrafactory and --out")
	gitignore, err := os.ReadFile(filepath.Join(out, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(gitignore), "git ignores the raw read in any repository")

	data, err := os.ReadFile(filepath.Join(out, rehearseReadFile))
	require.NoError(t, err)
	for _, value := range []string{readKeyID, readSecret, readToken} {
		assert.NotContains(t, string(data), value)
	}
	var read harness.AWSRecordedRead
	require.NoError(t, json.Unmarshal(data, &read))
	assert.Equal(t, readAccount, read.Account)
	assert.Equal(t, readRegion, read.Region)
	require.Len(t, read.Exchanges, 1)
	assert.Equal(t, "DescribeVpcs", read.Exchanges[0].Action)
	assert.JSONEq(t, `{"Filter.1.Name": "tag:Name", "Filter.1.Value.1": "web"}`, string(read.Exchanges[0].Params))
	assert.Equal(t, len(doer.actions), read.Calls)
}

// The account and region come from flags only: the config's aws: block
// is never read.
func TestRehearseReadTakesNoAccountOrRegionFromTheConfig(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"--account", "--region"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			_, repo := readRepo(t, readVPCEntry)
			// A config that loads, so a read that took its aws: block would get both.
			cfg := filepath.Join(repo, "infrafactory.yaml")
			mustWriteFile(t, cfg, "version: \"1.0\"\nagent:\n  type: claude-code\nmockway:\n  url: "+closedMockURL+
				"\naws:\n  account_id: \""+readAccount+"\"\n  region: "+readRegion+"\n")
			loaded, err := config.Load(cfg)
			require.NoError(t, err)
			require.Equal(t, config.AWSConfig{AccountID: readAccount, Region: readRegion}, loaded.AWS)
			doer := &readDoer{account: readAccount}

			run := runRead(t, repo, doer, append(readFlags(flag), "--config", cfg, "--out", readOutDir)...)

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeUsage, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), `"`+strings.TrimPrefix(flag, "--")+`"`)
			assert.Empty(t, doer.actions, "no request")
			assert.NoDirExists(t, filepath.Join(repo, readOutDir))
		})
	}
}

// Each refusal of the key or the read set stops the read before any
// read-set request, and writes no recorded read.
func TestRehearseReadRefusesBeforeTheReadSet(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		doer    *readDoer
		entries []string
		want    string
		sent    []string
	}{
		{
			name: "a key of another account", doer: &readDoer{account: "210987654321"},
			entries: []string{readVPCEntry},
			want:    `the key belongs to account "210987654321", not --account "` + readAccount + `"`,
			sent:    []string{"GetCallerIdentity"},
		},
		{
			name: "a DryRunOperation answer", doer: &readDoer{account: readAccount, dryRunCode: "DryRunOperation"},
			entries: []string{readVPCEntry},
			want:    harness.AWSReaderDryRunActions[0] + " answered DryRunOperation",
			sent:    append([]string{"GetCallerIdentity"}, harness.AWSReaderDryRunActions...),
		},
		{
			name: "an entry outside the reader table", doer: &readDoer{account: readAccount},
			entries: []string{readVPCEntry, `{"service": "ec2", "protocol": "query", "action": "CreateVpc", "params": {"CidrBlock": "10.0.0.0/16"}}`},
			want:    `entry 1 (service "ec2", protocol "query", action "CreateVpc") is not in the reader table`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, repo := readRepo(t, tc.entries...)

			run := runRead(t, repo, tc.doer, append(readFlags(), "--out", readOutDir)...)

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), tc.want)
			assert.Contains(t, run.stdout, tc.want)
			assert.Equal(t, tc.sent, tc.doer.actions)
			assert.NoFileExists(t, filepath.Join(repo, readOutDir, rehearseReadFile))
		})
	}
}

// An existing dir below .infrafactory on the way to --out, --out itself
// included, that is open to group or other is refused, never chmodded.
func TestRehearseReadRefusesADirOpenToOthers(t *testing.T) {
	t.Parallel()

	for _, open := range []string{readOutDir, filepath.Dir(readOutDir)} {
		t.Run(open, func(t *testing.T) {
			t.Parallel()
			_, repo := readRepo(t, readVPCEntry)
			require.NoError(t, os.MkdirAll(filepath.Join(repo, readOutDir), 0o700))
			require.NoError(t, os.Chmod(filepath.Join(repo, open), 0o750))
			doer := &readDoer{account: readAccount}

			run := runRead(t, repo, doer, append(readFlags(), "--out", readOutDir)...)

			require.Error(t, run.err)
			assert.Contains(t, run.err.Error(), "not a directory of mode 0700")
			assert.Empty(t, doer.actions, "no request")
			info, err := os.Stat(filepath.Join(repo, open))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o750), info.Mode().Perm())
		})
	}
}

// A recorded read that cannot be written after the account was read is
// reported as that, never as a refusal: the read happened.
func TestRehearseReadReportsAWriteFailureAfterTheRead(t *testing.T) {
	t.Parallel()

	const want = "the account was read into 1 exchanges, and the recorded read was not written: "
	for _, mode := range []OutputMode{OutputModeHuman, OutputModeJSON} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			_, repo := readRepo(t, readVPCEntry)
			// A dir where the file goes: the rename onto it fails, root or not.
			doer := &readDoer{account: readAccount, onDescribe: func() {
				require.NoError(t, os.Mkdir(filepath.Join(repo, readOutDir, rehearseReadFile), 0o700))
			}}

			run := runRead(t, repo, doer, append(readFlags(), "--out", readOutDir, "--output", string(mode))...)

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), want)
			assert.Contains(t, doer.actions, "DescribeVpcs")
			if mode == OutputModeHuman {
				assert.Contains(t, run.stdout, "Status: failed\nExchanges: 1\nNot written: "+want)
				assert.NotContains(t, run.stdout, "Refused")
				return
			}
			var result rehearseReadResult
			require.NoError(t, json.Unmarshal([]byte(run.stdout), &result))
			assert.Equal(t, CommandStatusFailed, result.Status)
			assert.Equal(t, 1, result.Exchanges)
			assert.Contains(t, result.WriteFailure, want)
			assert.Empty(t, result.Refusal)
			assert.Empty(t, result.Read)
		})
	}
}
