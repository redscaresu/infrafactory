package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// endPlanted is every file a test plants in a rehearsal before end, and
// endPlantedDirs the dirs they make that end must list too.
var (
	endPlanted = []string{
		"work/envs/prod/main.tf",
		"data/modules/modules.json",
		"state/terraform.tfstate",
		"state-config/terraform.tfstate",
		"fakeaws/fakeaws.db", "fakeaws/fakeaws.db-wal", "fakeaws/fakeaws.db-shm", "fakeaws/snapshot.db",
		"reads/recorded-read.json",
	}
	endPlantedDirs = []string{"work", "data", "state", "state-config", "fakeaws", "reads"}
)

// endStub starts the fakeaws stub in role (TestRehearsalFakeawsStub) in
// its own process group on a free loopback port with a fresh token, as
// start does but without start's checks, so a stub start refuses still
// runs. It returns the record end reads: pid, port and token. The group
// is killed when the test ends, unless it is gone already.
func endStub(t *testing.T, role string) rehearsalRecord {
	t.Helper()
	bin, _ := fakeawsStub(t, role)
	port, err := rehearsalFreePort()
	require.NoError(t, err)
	token := rehearsalToken()
	cmd := exec.Command(bin, "-bind", "127.0.0.1", "-port", strconv.Itoa(port), "-db", filepath.Join(t.TempDir(), "fakeaws.db"),
		"-record-requests", "-account", "123456789012", "-region", "eu-west-1")
	cmd.Env = []string{fakeawsAdminTokenEnv + "=" + token}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		select {
		case <-exited:
			return
		default:
		}
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			t.Errorf("stub group %d did not exit", pid)
		}
	})
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
		}
		return err == nil
	}, 30*time.Second, 20*time.Millisecond, "the stub never listened on %s", addr)
	select {
	case <-exited:
		t.Fatalf("the stub exited: something else holds %s", addr)
	default:
	}
	return rehearsalRecord{Schema: rehearsalRecordSchema, PID: pid, Port: port, Token: token}
}

// endRepo makes a repo holding rehearsal prepareID with endPlanted in it,
// and returns the repo and the rehearsal's dir.
func endRepo(t *testing.T) (string, string) {
	t.Helper()
	_, repo := prepareRepo(t, nil)
	dir := preparedDir(repo)
	for _, rel := range endPlanted {
		mustWriteFile(t, filepath.Join(dir, rel), rel)
	}
	return repo, dir
}

// endRoot is the root command with end built from e.
func endRoot(e rehearseEnd) *cobra.Command {
	root := NewRootCmd()
	group, _, _ := root.Find([]string{"rehearse"})
	end, _, _ := group.Find([]string{"end"})
	group.RemoveCommand(end)
	group.AddCommand(newRehearseEndCmd(e))
	root.SetErr(&bytes.Buffer{})
	return root
}

type endRun struct {
	stdout string
	result rehearseEndResult
	err    error
}

// runEnd runs `rehearse end id --output json` with e and decodes the
// report.
func runEnd(t *testing.T, e rehearseEnd, id string) endRun {
	t.Helper()
	root := endRoot(e)
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetArgs([]string{"rehearse", "end", id, "--output", "json"})
	run := endRun{err: root.Execute(), stdout: stdout.String()}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &run.result), run.stdout)
	return run
}

// assertPortRefuses fails unless a dial of the loopback port is refused.
func assertPortRefuses(t *testing.T, port int) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err == nil {
		conn.Close()
	}
	assert.ErrorIs(t, err, syscall.ECONNREFUSED, "port %d still answers", port)
}

// assertEnded fails unless end deleted the rehearsal at dir whole and
// listed every planted path, the record included when there was one.
func assertEnded(t *testing.T, run endRun, dir string, withRecord bool) {
	t.Helper()
	want := append(append([]string{}, endPlantedDirs...), endPlanted...)
	if withRecord {
		want = append(want, rehearsalRecordFile)
	}
	assert.Subset(t, run.result.Deleted, want)
	assert.NoDirExists(t, dir)
}

// With the stub recorded, end stops its group, its port refuses a dial,
// and every planted file is gone and listed. The token is never printed.
func TestRehearseEndStopsTheRecordedFakeawsAndDeletesEverything(t *testing.T) {
	t.Parallel()
	repo, dir := endRepo(t)
	record := endStub(t, "fakeaws")
	record.Account, record.Region = "123456789012", "eu-west-1"
	require.NoError(t, record.write(dir))

	run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	require.NoError(t, run.err, run.stdout)
	assert.Equal(t, CommandStatusSuccess, run.result.Status)
	assert.Equal(t, "stopped", run.result.Fakeaws)
	assert.False(t, groupAlive(record.PID), "the stub's group is still alive")
	assertPortRefuses(t, record.Port)
	assertEnded(t, run, dir, true)
	assert.NotContains(t, run.stdout, record.Token)
}

// A stub that ignores SIGTERM is stopped by the SIGKILL that follows the
// shortened fallback.
func TestRehearseEndKillsAFakeawsThatIgnoresSIGTERM(t *testing.T) {
	setForTest(t, &rehearsalFakeawsKillFallback, 500*time.Millisecond)
	repo, dir := endRepo(t)
	record := endStub(t, "ignore-term")
	require.NoError(t, record.write(dir))

	run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	require.NoError(t, run.err, run.stdout)
	assert.Equal(t, "stopped", run.result.Fakeaws)
	assert.False(t, groupAlive(record.PID), "the stub's group is still alive")
	assertPortRefuses(t, record.Port)
	assertEnded(t, run, dir, true)
}

// A token-guarded listener the test process holds, outside the stub's
// group, keeps answering after the group is signalled: end reports stop
// failed, exits 1 and deletes nothing. The group, gone after SIGTERM, is
// sent no SIGKILL: its id may be another group's by then.
func TestRehearseEndStopFailedDeletesNothing(t *testing.T) {
	setForTest(t, &rehearsalFakeawsKillFallback, 300*time.Millisecond)
	var sent []syscall.Signal
	setForTest(t, &rehearsalKillGroup, func(pgid int, sig syscall.Signal) error {
		if sig != 0 {
			sent = append(sent, sig)
		}
		return syscall.Kill(-pgid, sig)
	})
	repo, dir := endRepo(t)
	sleeper := endStub(t, "fakeaws")
	token := rehearsalToken()
	held := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mock/requests" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(held.Close)
	port := held.Listener.Addr().(*net.TCPAddr).Port
	record := rehearsalRecord{Schema: rehearsalRecordSchema, PID: sleeper.PID, Port: port, Token: token}
	require.NoError(t, record.write(dir))

	run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
	stopFailed := fmt.Sprintf("stop failed: pid %d port %d", sleeper.PID, port)
	assert.Equal(t, stopFailed, run.result.Fakeaws)
	assert.Contains(t, run.result.Refusal, stopFailed)
	assert.Equal(t, []syscall.Signal{syscall.SIGTERM}, sent)
	assert.Empty(t, run.result.Deleted)
	for _, rel := range append([]string{rehearsalRecordFile}, endPlanted...) {
		assert.FileExists(t, filepath.Join(dir, rel))
	}
}

// Unless the recorded fakeaws proves itself, end signals nothing, names
// why (and the pid when the record has one) and still deletes the
// rehearsal. Each case's sleeper is alive afterwards.
func TestRehearseEndSignalsNothingUnlessFakeawsProvesItself(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, role string
		// record writes the rehearsal's record for sleeper, or none, and
		// says whether the report can name the pid.
		record func(t *testing.T, dir string, sleeper rehearsalRecord) bool
		want   string
	}{
		{
			name: "missing record", role: "fakeaws",
			record: func(*testing.T, string, rehearsalRecord) bool { return false },
			want:   "cannot be read",
		},
		{
			name: "unparseable record", role: "fakeaws",
			record: func(t *testing.T, dir string, _ rehearsalRecord) bool {
				mustWriteFile(t, filepath.Join(dir, rehearsalRecordFile), "{")
				return false
			},
			want: "does not parse",
		},
		{
			name: "dead port", role: "fakeaws",
			record: func(t *testing.T, dir string, sleeper rehearsalRecord) bool {
				var err error
				sleeper.Port, err = rehearsalFreePort()
				require.NoError(t, err)
				require.NoError(t, sleeper.write(dir))
				return true
			},
			want: "/healthz failed",
		},
		{
			name: "healthz not 200", role: "never-healthy",
			record: func(t *testing.T, dir string, sleeper rehearsalRecord) bool {
				require.NoError(t, sleeper.write(dir))
				return true
			},
			want: "/healthz answered 503",
		},
		{
			name: "mock requests 401 for the record's token", role: "fakeaws",
			record: func(t *testing.T, dir string, sleeper rehearsalRecord) bool {
				sleeper.Token = rehearsalToken()
				require.NoError(t, sleeper.write(dir))
				return true
			},
			want: "GET /mock/requests with the record's token answered 401",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, dir := endRepo(t)
			sleeper := endStub(t, tc.role)
			namesPID := tc.record(t, dir, sleeper)

			run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
			require.NoError(t, run.err, run.stdout)
			assert.True(t, groupAlive(sleeper.PID), "the sleeper was signalled")
			assert.Contains(t, run.result.Fakeaws, "not signalled: ")
			assert.Contains(t, run.result.Fakeaws, tc.want)
			if namesPID {
				assert.Contains(t, run.result.Fakeaws, fmt.Sprintf("pid %d ", sleeper.PID))
			}
			assertEnded(t, run, dir, namesPID)
		})
	}
}

// A record whose pid is 0 or 1 signals nothing, though a fakeaws answers
// on its port with its token: kill(-0) reaches end's own group, kill(-1)
// every process the user owns. No signal is sent, a probe included.
func TestRehearseEndNeverSignalsPIDZeroOrOne(t *testing.T) {
	calls := killCalls(t, groupFound)
	setForTest(t, &rehearsalFakeawsKillFallback, 100*time.Millisecond)
	stub := endStub(t, "fakeaws")
	for _, pid := range []int{0, 1} {
		repo, dir := endRepo(t)
		record := stub
		record.PID = pid
		require.NoError(t, record.write(dir))

		run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
		require.NoError(t, run.err, run.stdout)
		assert.Contains(t, run.result.Fakeaws, "not signalled: the record names no fakeaws to signal", "pid %d", pid)
		assertEnded(t, run, dir, true)
	}
	assert.Empty(t, *calls)
	assert.True(t, groupAlive(stub.PID), "the stub was signalled")
}

// A record holding only pid, port and token, with no override files, as
// a prepare killed right after the start leaves it, is enough to stop
// the group.
func TestRehearseEndStopsTheFakeawsOfAPrepareKilledAfterTheStart(t *testing.T) {
	t.Parallel()
	_, repo := prepareRepo(t, nil)
	dir := preparedDir(repo)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	record := endStub(t, "fakeaws")
	require.NoError(t, record.write(dir))

	run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	require.NoError(t, run.err, run.stdout)
	assert.Equal(t, "stopped", run.result.Fakeaws)
	assert.False(t, groupAlive(record.PID), "the stub's group is still alive")
	assertPortRefuses(t, record.Port)
	assert.Equal(t, []string{rehearsalRecordFile}, run.result.Deleted)
	assert.NoDirExists(t, dir)
}

// A second end on the same id exits 0 with an empty deleted list.
func TestRehearseEndTwiceDeletesNothingTheSecondTime(t *testing.T) {
	t.Parallel()
	repo, dir := endRepo(t)

	first := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	require.NoError(t, first.err, first.stdout)
	assertEnded(t, first, dir, false)

	second := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	require.NoError(t, second.err, second.stdout)
	assert.Contains(t, second.stdout, `"deleted": []`)
	assert.Contains(t, second.result.Fakeaws, "no such rehearsal")
}

// A bad id, a symlinked <id> included, is a usage error that deletes
// nothing; end on a repo with no .infrafactory creates nothing.
func TestRehearseEndRefusesABadIDAndCreatesNothing(t *testing.T) {
	t.Parallel()
	_, repo := prepareRepo(t, nil)
	rehearsals := filepath.Join(repo, ".infrafactory", "rehearsals")
	outside := t.TempDir()
	sentinels := []string{
		filepath.Join(repo, ".infrafactory", "x", "sentinel"),
		filepath.Join(rehearsals, "a", "b", "sentinel"),
		filepath.Join(outside, "sentinel"),
	}
	for _, sentinel := range sentinels {
		mustWriteFile(t, sentinel, "keep")
	}
	require.NoError(t, os.Symlink(outside, filepath.Join(rehearsals, "link")))

	for _, id := range []string{"../x", "a/b", "link"} {
		run := runEnd(t, rehearseEnd{wd: repo}, id)
		assert.Equal(t, ExitCodeUsage, ExitCodeForError(run.err), "end %s", id)
		assert.Empty(t, run.result.Deleted, "end %s", id)
	}
	for _, sentinel := range sentinels {
		assert.FileExists(t, sentinel)
	}

	_, bare := prepareRepo(t, nil)
	run := runEnd(t, rehearseEnd{wd: bare}, "x")
	require.NoError(t, run.err, run.stdout)
	assert.Contains(t, run.result.Fakeaws, "no such rehearsal")
	assert.NoDirExists(t, filepath.Join(bare, ".infrafactory"))
}

// A cancel before the stop signals nothing and deletes nothing, exit 1.
func TestRehearseEndRefusesACancelBeforeTheStop(t *testing.T) {
	t.Parallel()
	repo, dir := endRepo(t)
	record := endStub(t, "fakeaws")
	require.NoError(t, record.write(dir))
	cancelled := func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		return ctx, cancel
	}

	run := runEnd(t, rehearseEnd{wd: repo, notify: cancelled}, prepareID)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
	assert.Contains(t, run.result.Refusal, "cancelled")
	assert.True(t, groupAlive(record.PID), "the stub was signalled")
	for _, rel := range append([]string{rehearsalRecordFile}, endPlanted...) {
		assert.FileExists(t, filepath.Join(dir, rel))
	}
}

// A dead proxy that accepts a dial does not stop end, which runs no
// tofu; the report names it.
func TestRehearseEndNamesALiveDeadProxy(t *testing.T) {
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { proxy.Close() })
	setForTest(t, &rehearsalDeadProxy, "http://"+proxy.Addr().String())
	repo, dir := endRepo(t)
	record := endStub(t, "fakeaws")
	require.NoError(t, record.write(dir))

	run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	require.NoError(t, run.err, run.stdout)
	assert.Equal(t, "stopped", run.result.Fakeaws)
	require.Len(t, run.result.Warnings, 1)
	assert.Contains(t, run.result.Warnings[0], proxy.Addr().String())
	assertEnded(t, run, dir, true)
}

// A removal that fails part way reports only what is gone, exit 1.
func TestRehearseEndReportsWhatIsGoneWhenTheRemovalFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes files from a read-only dir")
	}
	t.Parallel()
	repo, dir := endRepo(t)
	locked := filepath.Join(dir, "data", "modules")
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
	assert.Contains(t, run.result.Refusal, "cannot be removed whole")
	assert.Contains(t, run.result.Deleted, "work/envs/prod/main.tf")
	assert.NotContains(t, run.result.Deleted, "data/modules/modules.json")
	assert.FileExists(t, filepath.Join(locked, "modules.json"))
}
