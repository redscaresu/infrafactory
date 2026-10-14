package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/e2e"
)

// TestRehearsalTeardown runs a real prepare of wallsRoot under prodShell,
// then `rehearse read --out <id>/reads` of a stand-in prod fakeaws built
// from the same binary, and a token-guarded POST /mock/snapshot to the
// rehearsal's fakeaws. Every dir under <id> is then 0700, every provider
// binary 0700 and every other file 0600, the fakeaws db, its -wal and
// -shm and its snapshot among them.
//
// A user keeps a copy of <id>/work and <id>/data/providers, then
// `rehearse end` deletes <id>, listing what it held, and leaves the
// recorded port refusing a dial; a second end deletes nothing. In the
// kept copy, a bare `tofu init -plugin-dir` and `tofu apply` of the
// aws_lb under the prod shell dies at the closed fakeaws port or the dead
// proxy, and nothing reaches the shell's proxy. Nothing is ever written
// under .infrafactory/runs or live.
func TestRehearsalTeardown(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	// Before HOME moves, so the build uses this machine's Go caches.
	bin := fakeawsAtPin(t)

	repo := wallsRepo(t)
	proxy, shell := prodShell(t)
	noStores := func(stage string) {
		t.Helper()
		for _, dir := range []string{"runs", "live"} {
			assert.NoDirExists(t, filepath.Join(repo, ".infrafactory", dir), stage)
		}
	}

	var prepared struct {
		ID string `json:"id"`
	}
	out := rehearse(t, cli.NewRootCmd(), "prepare", wallsRootDir, "--account", wallsAccount, "--region", assumptionRegion, "--fakeaws-bin", bin)
	require.NoError(t, json.Unmarshal(out, &prepared), string(out))
	rehearsal := filepath.Join(repo, ".infrafactory", "rehearsals", prepared.ID)
	record := readPrepareRecord(t, rehearsal)
	require.Len(t, record.Roots, 1)
	noStores("after prepare")

	// The recorded read and the fakeaws files, relative to <id>.
	read := "reads/read.json"
	held := []string{read, "fakeaws/fakeaws.db", "fakeaws/fakeaws.db-wal", "fakeaws/fakeaws.db-shm", "fakeaws/fakeaws.db.snapshot"}
	var readResult struct {
		Read      string `json:"read"`
		Exchanges int    `json:"exchanges"`
	}
	out = rehearse(t, cli.RehearseReadRoot(startStandInProd(t, bin)), teardownReadArgs(t, filepath.Join(rehearsal, "reads"))...)
	require.NoError(t, json.Unmarshal(out, &readResult), string(out))
	assert.Equal(t, filepath.Join(rehearsal, read), readResult.Read)
	assert.Equal(t, 1, readResult.Exchanges)
	snapshotFakeaws(t, record.Port, record.Token)
	noStores("after read and snapshot")

	for _, rel := range held {
		require.FileExists(t, filepath.Join(rehearsal, rel), "the walk must meet it")
	}
	assertTightModes(t, rehearsal)

	kept := realTempDir(t)
	require.NoError(t, os.CopyFS(filepath.Join(kept, "work"), os.DirFS(filepath.Join(rehearsal, "work"))))
	require.NoError(t, os.CopyFS(filepath.Join(kept, "providers"), os.DirFS(filepath.Join(rehearsal, "data", "providers"))))

	ended := endRehearsal(t, prepared.ID)
	assert.Equal(t, "stopped", ended.Fakeaws)
	assert.Subset(t, ended.Deleted, append(held, "state", "work", record.Roots[0].Root))
	assert.NoDirExists(t, rehearsal)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", record.Port), time.Second)
	if err == nil {
		conn.Close()
	}
	assert.ErrorIs(t, err, syscall.ECONNREFUSED, "the recorded port must refuse a dial after end")
	again := endRehearsal(t, prepared.ID)
	assert.Empty(t, again.Deleted, "a second end deleted something")
	noStores("after end")

	// The user's kept copy, with the prod shell's keys, profile and
	// proxies.
	w := wallsDir{t: t, dir: filepath.Join(kept, record.Roots[0].Root), proxy: proxy}
	initOut, err := w.tofu(shell, "init", "-input=false", "-no-color", "-plugin-dir="+filepath.Join(kept, "providers"))
	require.NoError(t, err, initOut)
	dead, err := url.Parse(cli.RehearsalDeadProxy)
	require.NoError(t, err)
	w.dies(shell, regexp.MustCompile(fmt.Sprintf(`dial tcp 127\.0\.0\.1:(%d|%s): connect: connection refused`, record.Port, dead.Port())), wallsApplyLB...)
	assert.Zero(t, proxy.count(), "a request reached the shell's proxy: %v", proxy.requests())
	noStores("after the bare apply")
}

// rehearse runs `rehearse args --output json` on root and returns its
// stdout; an error fails the test.
func rehearse(t *testing.T, root *cobra.Command, args ...string) []byte {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(append(append([]string{"rehearse"}, args...), "--output", "json"))
	require.NoError(t, root.Execute(), "rehearse %s\n%s\n%s", strings.Join(args, " "), stdout, stderr)
	return stdout.Bytes()
}

// endReport is what the teardown test reads of `rehearse end`'s report.
type endReport struct {
	Fakeaws string   `json:"fakeaws"`
	Deleted []string `json:"deleted"`
}

// endRehearsal runs `rehearse end id`, which must exit 0.
func endRehearsal(t *testing.T, id string) endReport {
	t.Helper()
	out := rehearse(t, cli.NewRootCmd(), "end", id)
	var report endReport
	require.NoError(t, json.Unmarshal(out, &report), string(out))
	return report
}

// teardownReadArgs are `rehearse read` and its flags, reading the stand-in
// prod's identity into out with a fake reader key.
func teardownReadArgs(t *testing.T, out string) []string {
	t.Helper()
	dir := t.TempDir()
	credentials, readSet := filepath.Join(dir, "reader.env"), filepath.Join(dir, "read-set.json")
	require.NoError(t, os.WriteFile(credentials, []byte("AWS_ACCESS_KEY_ID="+fakeAccessKeyID+"\nAWS_SECRET_ACCESS_KEY="+fakeSecretAccessKey+"\n"), 0o600))
	require.NoError(t, os.WriteFile(readSet, []byte(`[{"service": "sts", "protocol": "query", "action": "GetCallerIdentity", "params": {}}]`), 0o600))
	return []string{"read", "--account", wallsAccount, "--region", assumptionRegion, "--credentials", credentials, "--read-set", readSet, "--out", out}
}

// snapshotFakeaws has the fakeaws on the loopback port snapshot its db,
// with token as the bearer, through no proxy.
func snapshotFakeaws(t *testing.T, port int, token string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/mock/snapshot", port), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := standInClient().Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// standInClient sends to loopback through no proxy, under a deadline.
func standInClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{}}
}

// standInProd is a doer that sends each request `rehearse read` makes to
// AWS to a fakeaws standing in for prod, at its service's fakeaws path.
// fakeaws does not model DryRun, so a DryRun request, the preflight's, is
// answered UnauthorizedOperation as a reader key's is, and never sent:
// its placeholder requests are writes.
type standInProd struct{ url string }

// startStandInProd starts a fakeaws from bin as the stand-in prod, and
// stops it when the test ends.
func startStandInProd(t *testing.T, bin string) standInProd {
	t.Helper()
	pid, port, err := cli.StartRehearsalFakeaws(bin, filepath.Join(t.TempDir(), "prod"), wallsAccount, assumptionRegion)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, cli.StopRehearsalFakeaws(pid, port)) })
	return standInProd{url: fmt.Sprintf("http://127.0.0.1:%d", port)}
}

func (p standInProd) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	if form, err := url.ParseQuery(string(body)); err == nil && form.Get("DryRun") == "true" {
		return &http.Response{
			StatusCode: http.StatusForbidden, Header: http.Header{"Content-Type": {"text/xml"}}, Request: req,
			Body: io.NopCloser(strings.NewReader(cli.EC2ErrorAnswer("UnauthorizedOperation"))),
		}, nil
	}
	svc, _, _ := strings.Cut(req.URL.Host, ".")
	svc = strings.ToUpper(svc)
	if _, ok := cli.AWSFakeawsPaths[svc]; !ok {
		return nil, fmt.Errorf("the stand-in prod serves no %s", req.URL.Host)
	}
	target, err := url.Parse(p.url + strings.TrimSuffix(fakeawsPath(svc)+req.URL.Path, "/"))
	if err != nil {
		return nil, err
	}
	out := req.Clone(req.Context())
	out.URL, out.Host = target, ""
	out.Body, out.ContentLength = io.NopCloser(bytes.NewReader(body)), int64(len(body))
	return standInClient().Do(out)
}
