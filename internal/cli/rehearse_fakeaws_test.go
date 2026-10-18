package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// fakeawsStubEnv selects TestRehearsalFakeawsStub's role; fakeawsStubDirEnv
// names the dir its script records into.
const (
	fakeawsStubEnv    = "INFRAFACTORY_FAKEAWS_STUB"
	fakeawsStubDirEnv = "INFRAFACTORY_FAKEAWS_STUB_DIR"
)

// fakeawsShellEnv is what /bin/sh itself adds to the environment its
// `env` sees, whatever it was started with.
var fakeawsShellEnv = []string{"PWD", "SHLVL", "_"}

// TestRehearsalFakeawsStub is not a test: it is the fakeaws a stub script
// execs into. It takes fakeaws's flags after "--" and serves /healthz and
// /mock/requests on -bind:-port as its role says, and /exit, after which
// it exits 1 on its own. "fakeaws" behaves as
// fakeaws does; "unknown-flag" knows no -record-requests; "no-auth"
// answers /mock/requests without the token; "no-record" answers it 501,
// as fakeaws does with recording off; "never-healthy" answers /healthz
// 503; "all-interfaces" listens on 0.0.0.0; "bind-fail-once" and
// "bind-fail" fail to bind on their first or on every try; "ignore-term"
// ignores SIGTERM; "prints-token" logs its token and exits 1;
// "exits-after-checks" exits 3 once start's last request is answered; "zero-stop"
// only stops a zero rehearsalFakeaws; "late-healthy" answers /healthz 503
// until its stub dir holds a file named healthy; "starter" is prepare, not
// fakeaws: it starts the stub at the first arg after "--" and writes its
// pid, port and token to the second, then exits 0; "walls" is prepare's
// walls, with the stub at the first arg after "--" as its binary and the
// second as the rehearsal, and exits 1 if they ever return.
func TestRehearsalFakeawsStub(t *testing.T) {
	role := os.Getenv(fakeawsStubEnv)
	if role == "" {
		return
	}
	if role == "starter" {
		bin, ready := flag.Arg(0), flag.Arg(1)
		f, err := rehearsalStartFakeaws(context.Background(), rehearsalFakeawsOpts{
			bin: bin, dir: filepath.Join(filepath.Dir(ready), "fakeaws"), account: "123456789012", region: "eu-west-1",
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if os.WriteFile(ready+".tmp", fmt.Appendf(nil, "%d %d %s", f.PID, f.Port, f.Token), 0o600) != nil ||
			os.Rename(ready+".tmp", ready) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if role == "walls" {
		p := rehearsePrepare{
			runner: harness.CommandRunnerFunc(func(context.Context, harness.Command) (harness.CommandResult, error) {
				return harness.CommandResult{Stdout: []byte(prepareSchema)}, nil
			}),
			fakeawsBin:   func(context.Context, harness.CommandRunner, string) (string, error) { return flag.Arg(0), nil },
			startFakeaws: rehearsalStartFakeawsWithStop,
		}
		_, err := p.walls(context.Background(), flag.Arg(1), rehearsalRecord{
			Account: "123456789012", Region: "eu-west-1", Roots: []rehearsalRoot{{Copy: "work", Root: "work"}},
		})
		fmt.Fprintln(os.Stderr, "walls returned:", err)
		os.Exit(1)
	}
	if role == "zero-stop" {
		if (rehearsalFakeaws{}).stop() != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	fs := flag.NewFlagSet("fakeaws", flag.ContinueOnError)
	bind := fs.String("bind", "", "")
	port := fs.Int("port", 0, "")
	fs.String("db", "", "")
	fs.String("account", "", "")
	fs.String("region", "", "")
	fs.String("admin-token", "", "")
	if role != "unknown-flag" {
		fs.Bool("record-requests", false, "")
	}
	if fs.Parse(flag.Args()) != nil {
		os.Exit(2)
	}
	host := *bind
	if role == "all-interfaces" {
		host = "0.0.0.0"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(*port))
	if role == "prints-token" {
		fmt.Fprintf(os.Stderr, "%s=%s\n", fakeawsAdminTokenEnv, os.Getenv(fakeawsAdminTokenEnv))
		os.Exit(1)
	}
	if role == "bind-fail" || role == "bind-fail-once" && len(fakeawsStubPIDs(os.Getenv(fakeawsStubDirEnv))) == 1 {
		fmt.Fprintf(os.Stderr, "fakeaws: serve: listen tcp %s: bind: %s\n", addr, fakeawsBindFailure)
		os.Exit(1)
	}
	if role == "ignore-term" {
		signal.Ignore(syscall.SIGTERM)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakeaws: serve: %v\n", err)
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/exit", func(http.ResponseWriter, *http.Request) {
		go func() {
			time.Sleep(50 * time.Millisecond)
			os.Exit(1)
		}()
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if role == "never-healthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if _, err := os.Stat(filepath.Join(os.Getenv(fakeawsStubDirEnv), "healthy")); role == "late-healthy" && err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	mux.HandleFunc("/mock/requests", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case role == "no-record":
			w.WriteHeader(http.StatusNotImplemented)
		case role != "no-auth" && r.Header.Get("Authorization") != "Bearer "+os.Getenv(fakeawsAdminTokenEnv):
			w.WriteHeader(http.StatusUnauthorized)
			// Another token is start's last request.
			if role == "exits-after-checks" && r.Header.Get("Authorization") != "" {
				go func() {
					time.Sleep(50 * time.Millisecond)
					os.Exit(3)
				}()
			}
		}
	})
	_ = http.Serve(ln, mux)
	os.Exit(1)
}

// writeScript writes an executable script at path from a child process, so
// this process never holds a write fd on it: a parallel test's fork would
// inherit that fd, and exec of the script then fails with "text file busy"
// (golang/go#22315).
func writeScript(t *testing.T, path, body string) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", `cat > "$1" && chmod 0700 "$1"`, "sh", path)
	cmd.Stdin = strings.NewReader(body)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "write %s: %s", path, out)
}

// fakeawsStub writes a 0700 /bin/sh script that records its pid, argv and
// env into a dir of its own, then execs the test binary as
// TestRehearsalFakeawsStub in role, exporting the role only after env is
// recorded. exec keeps the pid, so the stub stays its group's leader. It
// returns the script and the dir.
func fakeawsStub(t *testing.T, role string) (string, string) {
	t.Helper()
	testBin, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
d=%s
echo $$ >> "$d/pids"
printf '%%s\n' "$@" > "$d/args"
/usr/bin/env > "$d/env"
export %s=%s %s="$d"
exec %s '-test.run=^TestRehearsalFakeawsStub$' -- "$@"
`, shellQuote(dir), fakeawsStubEnv, role, fakeawsStubDirEnv, shellQuote(testBin))
	path := filepath.Join(dir, "fakeaws")
	writeScript(t, path, script)
	return path, dir
}

// fakeawsStubPIDs is the pid of each start of the stub recording into dir.
func fakeawsStubPIDs(dir string) []int {
	b, _ := os.ReadFile(filepath.Join(dir, "pids"))
	var pids []int
	for _, line := range strings.Fields(string(b)) {
		pid, _ := strconv.Atoi(line)
		pids = append(pids, pid)
	}
	return pids
}

// fakeawsStubOpts starts bin with its dir under a fresh temp dir.
func fakeawsStubOpts(t *testing.T, bin string) rehearsalFakeawsOpts {
	return rehearsalFakeawsOpts{
		bin: bin, dir: filepath.Join(t.TempDir(), "fakeaws"), account: "123456789012", region: "eu-west-1",
	}
}

// groupAlive says whether any process is left in process group pgid.
func groupAlive(pgid int) bool {
	return syscall.Kill(-pgid, 0) == nil
}

// requireNoStubLeft fails unless every group the stub started is gone.
func requireNoStubLeft(t *testing.T, dir string) {
	t.Helper()
	pids := fakeawsStubPIDs(dir)
	require.NotEmpty(t, pids, "the stub never started")
	for _, pid := range pids {
		assert.False(t, groupAlive(pid), "stub group %d is still alive", pid)
	}
}

// setForTest sets *p to v until the test ends.
func setForTest[T any](t *testing.T, p *T, v T) {
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// shortenFakeawsWaits sets the readiness deadline, and shortens the kill
// fallback and the exit grace, for one test.
func shortenFakeawsWaits(t *testing.T, ready time.Duration) {
	setForTest(t, &rehearsalFakeawsReady, ready)
	setForTest(t, &rehearsalFakeawsKillFallback, 500*time.Millisecond)
	setForTest(t, &rehearsalFakeawsExitGrace, 200*time.Millisecond)
}

// stopOnCleanup stops f when the test ends; a stop that fails fails it.
func stopOnCleanup(t *testing.T, f rehearsalFakeaws) {
	t.Cleanup(func() { assert.NoError(t, f.stop()) })
}

// fakeawsGet is the status of GET /mock/requests on port, with token.
func fakeawsGet(t *testing.T, port int, token string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, err := rehearsalFakeawsStatus(ctx, &http.Client{Transport: &http.Transport{DisableKeepAlives: true}},
		fmt.Sprintf("http://127.0.0.1:%d/mock/requests", port), token)
	require.NoError(t, err)
	return code
}

func TestRehearsalStartFakeawsStartsItAsOurs(t *testing.T) {
	bin, stubDir := fakeawsStub(t, "fakeaws")
	opts := fakeawsStubOpts(t, bin)

	f, err := rehearsalStartFakeaws(context.Background(), opts)
	require.NoError(t, err)
	stopOnCleanup(t, f)

	args, err := os.ReadFile(filepath.Join(stubDir, "args"))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"-bind", "127.0.0.1", "-port", strconv.Itoa(f.Port), "-db", filepath.Join(opts.dir, "fakeaws.db"),
		"-record-requests", "-account", "123456789012", "-region", "eu-west-1",
	}, strings.Split(strings.TrimSuffix(string(args), "\n"), "\n"))
	assert.NotContains(t, string(args), f.Token)
	env, err := os.ReadFile(filepath.Join(stubDir, "env"))
	require.NoError(t, err)
	got := slices.DeleteFunc(strings.Split(strings.TrimSuffix(string(env), "\n"), "\n"), func(kv string) bool {
		return slices.Contains(fakeawsShellEnv, strings.SplitN(kv, "=", 2)[0])
	})
	// The name fakeaws reads, spelled out: fakeawsAdminTokenEnv is what is under test.
	assert.Equal(t, []string{"FAKEAWS_ADMIN_TOKEN=" + f.Token}, got)
	assert.Len(t, f.Token, 64)

	pgid, err := syscall.Getpgid(f.PID)
	require.NoError(t, err)
	assert.Equal(t, f.PID, pgid, "fakeaws leads its own process group")
	assert.Equal(t, http.StatusOK, fakeawsGet(t, f.Port, f.Token))
	for path, mode := range map[string]os.FileMode{opts.dir: 0o700, filepath.Join(opts.dir, "fakeaws.log"): 0o600} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, mode, info.Mode().Perm(), path)
	}
}

func TestRehearsalStartFakeawsOutlivesCtx(t *testing.T) {
	bin, _ := fakeawsStub(t, "fakeaws")
	ctx, cancel := context.WithCancel(context.Background())
	f, err := rehearsalStartFakeaws(ctx, fakeawsStubOpts(t, bin))
	require.NoError(t, err)
	stopOnCleanup(t, f)

	cancel()
	assert.Never(t, func() bool { return !groupAlive(f.PID) }, time.Second, 20*time.Millisecond,
		"fakeaws died with the ctx that started it")
	require.NoError(t, f.stop())
	assert.False(t, groupAlive(f.PID), "fakeaws outlived stop")
}

// fakeaws must outlive the prepare that started it, for the passthroughs
// and `rehearse end` (ADR-0042 § Start): a death signal, such as
// childSysProcAttr's on Linux, would kill it as prepare exits.
func TestRehearsalStartFakeawsOutlivesItsStarter(t *testing.T) {
	bin, stubDir := fakeawsStub(t, "fakeaws")
	ready := filepath.Join(t.TempDir(), "ready")
	// Past the starter's own ready wait, so a stall is the starter's, not fakeaws's.
	ctx, cancel := context.WithTimeout(context.Background(), rehearsalFakeawsReady+15*time.Second)
	defer cancel()
	starter := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRehearsalFakeawsStub$", "--", bin, ready)
	starter.Env = append(os.Environ(), fakeawsStubEnv+"=starter")
	starter.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	starter.Cancel = func() error { return syscall.Kill(-starter.Process.Pid, syscall.SIGKILL) }
	starter.WaitDelay = 5 * time.Second
	// fakeaws leads a group of its own, so a starter that never reported
	// it leaves each group the stub recorded to be killed here.
	recorded := false
	t.Cleanup(func() {
		for _, pid := range fakeawsStubPIDs(stubDir) {
			if !recorded && groupAlive(pid) {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		}
	})
	out, err := starter.CombinedOutput()
	require.NoError(t, err, "the starter failed: %s", out)
	raw, err := os.ReadFile(ready)
	require.NoError(t, err)
	var f rehearsalFakeaws
	_, err = fmt.Sscanf(string(raw), "%d %d %s", &f.PID, &f.Port, &f.Token)
	require.NoError(t, err)
	f.gone = new(atomic.Bool)
	recorded = true
	stopOnCleanup(t, f)

	client := rehearsalFakeawsClient()
	url := fmt.Sprintf("http://127.0.0.1:%d/mock/requests", f.Port)
	assert.Never(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		code, err := rehearsalFakeawsStatus(ctx, client, url, f.Token)
		return err != nil || code != http.StatusOK
	}, 2*time.Second, 50*time.Millisecond, "fakeaws died with the process that started it")
}

func TestRehearsalFakeawsStopKillsAGroupIgnoringSIGTERM(t *testing.T) {
	shortenFakeawsWaits(t, 30*time.Second)
	bin, _ := fakeawsStub(t, "ignore-term")
	f, err := rehearsalStartFakeaws(context.Background(), fakeawsStubOpts(t, bin))
	require.NoError(t, err)
	stopOnCleanup(t, f)

	require.NoError(t, f.stop())
	assert.False(t, groupAlive(f.PID), "fakeaws outlived stop")
}

// The zero value is stopped in a child leading its own group, so a stop
// that signalled its caller's group ends only that child.
func TestRehearsalFakeawsZeroStopSignalsNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRehearsalFakeawsStub$")
	cmd.Env = append(os.Environ(), fakeawsStubEnv+"=zero-stop")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, "%s", out)
}

func TestRehearsalStartFakeawsRefuses(t *testing.T) {
	for _, tc := range []struct {
		role  string
		want  []string
		tries int
	}{
		{"unknown-flag", []string{"exited before it was ready", "exit status 2", "flag provided but not defined: -record-requests"}, 1},
		{"no-auth", []string{"GET /mock/requests without its token answered 200, not 401"}, 1},
		{"no-record", []string{"GET /mock/requests with its token answered 501, not 200"}, 1},
		{"never-healthy", []string{"not ready within 10s", "/healthz answered 503"}, 1},
		{"bind-fail", []string{"3 tries in all", fakeawsBindFailure}, 3},
		{"prints-token", []string{"exit status 1", "withheld: it holds the admin token"}, 1},
	} {
		t.Run(tc.role, func(t *testing.T) {
			shortenFakeawsWaits(t, 10*time.Second)
			bin, stubDir := fakeawsStub(t, tc.role)

			f, err := rehearsalStartFakeaws(context.Background(), fakeawsStubOpts(t, bin))
			stopOnCleanup(t, f)
			require.Error(t, err)
			for _, want := range tc.want {
				assert.Contains(t, err.Error(), want)
			}
			assert.Len(t, fakeawsStubPIDs(stubDir), tc.tries, "tries")
			requireNoStubLeft(t, stubDir)
			env, readErr := os.ReadFile(filepath.Join(stubDir, "env"))
			require.NoError(t, readErr)
			_, token, ok := strings.Cut(string(env), "FAKEAWS_ADMIN_TOKEN=")
			require.True(t, ok, "the stub was given no token")
			assert.NotContains(t, err.Error(), strings.Fields(token)[0], "the refusal prints the token")
		})
	}
}

func TestRehearsalStartFakeawsRefusesAllInterfaces(t *testing.T) {
	ips, err := rehearsalNonLoopbackIPs()
	require.NoError(t, err)
	if len(ips) == 0 {
		t.Skip("this host has no non-loopback address, so a listener on 0.0.0.0 cannot be told from one on 127.0.0.1")
	}
	bin, stubDir := fakeawsStub(t, "all-interfaces")

	f, err := rehearsalStartFakeaws(context.Background(), fakeawsStubOpts(t, bin))
	stopOnCleanup(t, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not only on loopback")
	requireNoStubLeft(t, stubDir)
}

// Each try is passed to started, the one admitted last.
func TestRehearsalStartFakeawsRetriesABindFailure(t *testing.T) {
	bin, stubDir := fakeawsStub(t, "bind-fail-once")
	opts := fakeawsStubOpts(t, bin)
	var started []rehearsalFakeaws
	opts.started = func(f rehearsalFakeaws) error {
		started = append(started, f)
		return nil
	}

	f, err := rehearsalStartFakeaws(context.Background(), opts)
	require.NoError(t, err)
	stopOnCleanup(t, f)
	assert.Len(t, fakeawsStubPIDs(stubDir), 2, "admitted on the second try")
	assert.Equal(t, http.StatusOK, fakeawsGet(t, f.Port, f.Token))
	require.Len(t, started, 2)
	last := started[1]
	assert.Equal(t, [3]any{f.PID, f.Port, f.Token}, [3]any{last.PID, last.Port, last.Token})
	assert.NotEqual(t, started[0].Token, last.Token)
}

// A try started cannot record is stopped and refused, never retried.
func TestRehearsalStartFakeawsRefusesATryItCannotRecord(t *testing.T) {
	bin, _ := fakeawsStub(t, "fakeaws")
	opts := fakeawsStubOpts(t, bin)
	var started []rehearsalFakeaws
	opts.started = func(f rehearsalFakeaws) error {
		started = append(started, f)
		return errors.New("disk full")
	}

	f, err := rehearsalStartFakeaws(context.Background(), opts)
	stopOnCleanup(t, f)
	require.Error(t, err)
	assert.Equal(t, "fakeaws cannot be recorded: disk full", err.Error())
	require.Len(t, started, 1)
	assert.False(t, groupAlive(started[0].PID), "the try it could not record is still alive")
}

func TestRehearsalStartFakeawsCancelledWhileWaiting(t *testing.T) {
	bin, stubDir := fakeawsStub(t, "never-healthy")
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel once the stub has run, at most 10s on.
	go func() {
		defer cancel()
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if len(fakeawsStubPIDs(stubDir)) > 0 {
				return
			}
		}
	}()

	f, err := rehearsalStartFakeaws(ctx, fakeawsStubOpts(t, bin))
	stopOnCleanup(t, f)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "%v", err)
	requireNoStubLeft(t, stubDir)
}

// The first port start picks is held by another server, which answers
// /healthz 200 and /mock/requests 401 to every token or 200 to any bearer
// token. The stub fails to bind there and exits, and is retried on a free
// port whatever that server answered.
func TestRehearsalStartFakeawsRetriesPastAPortSquatter(t *testing.T) {
	for name, mockRequests := range map[string]http.HandlerFunc{
		"refusing every token": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
		"taking any bearer token": func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
			}
		},
	} {
		t.Run(name, func(t *testing.T) { retriesPastASquatter(t, mockRequests) })
	}
}

func retriesPastASquatter(t *testing.T, mockRequests http.HandlerFunc) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(http.ResponseWriter, *http.Request) {})
	mux.Handle("/mock/requests", mockRequests)
	// Every Authorization the squatter is sent.
	var mu sync.Mutex
	var seen []string
	squatter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(squatter.Close)
	squatPort := squatter.Listener.Addr().(*net.TCPAddr).Port
	freePort, picks := rehearsalFreePort, 0
	setForTest(t, &rehearsalFreePort, func() (int, error) {
		if picks++; picks == 1 {
			return squatPort, nil
		}
		return freePort()
	})
	// Long enough for the stub to reach its bind on a loaded host.
	setForTest(t, &rehearsalFakeawsExitGrace, 30*time.Second)
	bin, stubDir := fakeawsStub(t, "fakeaws")

	f, err := rehearsalStartFakeaws(context.Background(), fakeawsStubOpts(t, bin))
	require.NoError(t, err)
	stopOnCleanup(t, f)
	assert.Len(t, fakeawsStubPIDs(stubDir), 2, "admitted on the second try")
	assert.NotEqual(t, squatPort, f.Port)
	assert.Equal(t, http.StatusOK, fakeawsGet(t, f.Port, f.Token))
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, strings.Join(seen, "\n"), "Bearer ", "the squatter was never sent a token")
	assert.NotContains(t, seen, "Bearer "+f.Token, "the squatter was sent the admitted fakeaws's token")
}

// killCalls swaps rehearsalKillGroup, for the rest of the test, for one
// that records the signal of every call, a probe (signal 0) included, and
// sends none: a probe is answered by probe, any other signal by nil.
func killCalls(t *testing.T, probe func(pgid int) error) *[]syscall.Signal {
	var calls []syscall.Signal
	setForTest(t, &rehearsalKillGroup, func(pgid int, sig syscall.Signal) error {
		calls = append(calls, sig)
		if sig == 0 {
			return probe(pgid)
		}
		return nil
	})
	return &calls
}

// realProbe probes the group for real; groupFound answers every probe as
// a group would whose id the system has reused.
func realProbe(pgid int) error { return syscall.Kill(-pgid, 0) }
func groupFound(int) error     { return nil }

// waitExited waits until f's process has exited and been reaped.
func waitExited(t *testing.T, f rehearsalFakeaws) {
	t.Helper()
	select {
	case <-f.exited:
	case <-time.After(10 * time.Second):
		t.Fatal("the stub was never reaped")
	}
}

// startedStub starts the plain stub and stops it when the test ends.
func startedStub(t *testing.T) rehearsalFakeaws {
	t.Helper()
	bin, _ := fakeawsStub(t, "fakeaws")
	f, err := rehearsalStartFakeaws(context.Background(), fakeawsStubOpts(t, bin))
	require.NoError(t, err)
	stopOnCleanup(t, f)
	return f
}

func TestRehearsalFakeawsStopSendsNothingToAGoneGroup(t *testing.T) {
	t.Run("fakeaws exited on its own", func(t *testing.T) {
		setForTest(t, &rehearsalFakeawsKillFallback, 500*time.Millisecond)
		f := startedStub(t)
		code, err := rehearsalFakeawsStatus(context.Background(), &http.Client{Timeout: 5 * time.Second},
			fmt.Sprintf("http://127.0.0.1:%d/exit", f.Port), "")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code)
		waitExited(t, f)

		calls := killCalls(t, groupFound)
		assert.NoError(t, f.stop())
		assert.Empty(t, *calls, "stop probed or signalled a reaped fakeaws's group id")
	})
	// A value without exited, as a hand-made one is, keeps the probe.
	t.Run("a second stop, without exited", func(t *testing.T) {
		setForTest(t, &rehearsalFakeawsKillFallback, 500*time.Millisecond)
		h := startedStub(t)
		h.exited = nil
		require.NoError(t, h.stop())

		calls := killCalls(t, groupFound)
		assert.NoError(t, h.stop())
		assert.Empty(t, *calls)
	})
	t.Run("a group already gone, without exited", func(t *testing.T) {
		f := startedStub(t)
		require.NoError(t, syscall.Kill(-f.PID, syscall.SIGKILL))
		waitExited(t, f)
		h := f
		h.exited = nil

		calls := killCalls(t, realProbe)
		assert.NoError(t, h.stop())
		assert.Equal(t, []syscall.Signal{0}, *calls, "one probe and no signal")
	})
}

// With every signal dropped, the group outlives SIGKILL as one in
// uninterruptible sleep would.
func TestRehearsalFakeawsStopFailsWhenTheGroupSurvives(t *testing.T) {
	t.Run("stop", func(t *testing.T) {
		setForTest(t, &rehearsalFakeawsKillFallback, 300*time.Millisecond)
		bin, _ := fakeawsStub(t, "fakeaws")
		f, err := rehearsalStartFakeaws(context.Background(), fakeawsStubOpts(t, bin))
		require.NoError(t, err)
		stopOnCleanup(t, f) // the real stop, once killCalls is undone

		sent := killCalls(t, realProbe)
		err = f.stop()
		require.Error(t, err)
		assert.Equal(t, fmt.Sprintf("stop failed: pid %d port %d", f.PID, f.Port), err.Error())
		assert.Equal(t, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}, *sent)
	})
	t.Run("a refusal names it", func(t *testing.T) {
		shortenFakeawsWaits(t, 10*time.Second)
		bin, stubDir := fakeawsStub(t, "no-auth")
		t.Cleanup(func() {
			for _, pid := range fakeawsStubPIDs(stubDir) {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		})
		killCalls(t, realProbe)

		_, err := rehearsalStartFakeaws(context.Background(), fakeawsStubOpts(t, bin))
		require.Error(t, err)
		pids := fakeawsStubPIDs(stubDir)
		require.Len(t, pids, 1)
		assert.Contains(t, err.Error(), "answered 200, not 401")
		assert.Contains(t, err.Error(), fmt.Sprintf("stop failed: pid %d port ", pids[0]))
	})
}

// The stub exits once its HTTP checks are answered, and the last check, the
// address listing, returns only after it is reaped: a start that passed
// its checks is refused when its child is gone.
func TestRehearsalStartFakeawsRefusesAChildGoneAfterItsChecks(t *testing.T) {
	shortenFakeawsWaits(t, 10*time.Second)
	bin, stubDir := fakeawsStub(t, "exits-after-checks")
	setForTest(t, &rehearsalNonLoopbackIPs, func() ([]net.IP, error) {
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if pids := fakeawsStubPIDs(stubDir); len(pids) > 0 && errors.Is(syscall.Kill(pids[0], 0), syscall.ESRCH) {
				// Reaped; exited closes just after.
				time.Sleep(100 * time.Millisecond)
				break
			}
		}
		return nil, nil
	})

	f, err := rehearsalStartFakeaws(context.Background(), fakeawsStubOpts(t, bin))
	stopOnCleanup(t, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fakeaws exited before it was ready (exit status 3)")
	assert.Len(t, fakeawsStubPIDs(stubDir), 1, "tries")
}
