package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// rehearsalFakeawsReady bounds start's wait for a started fakeaws to pass
// its live checks. A var only so a test can shorten it.
var rehearsalFakeawsReady = 30 * time.Second

// rehearsalFakeawsKillFallback is how long stop waits after SIGTERM for the
// process group to go before it sends SIGKILL, and then how long it waits
// again. A var only so a test can shorten it.
var rehearsalFakeawsKillFallback = 5 * time.Second

// rehearsalFakeawsExitGrace is how long a refusal waits for a child that
// is failing on its own to exit before it is stopped, so its exit and log,
// not what answered on its port, decide the refusal. A var only so a test
// can change it.
var rehearsalFakeawsExitGrace = 2 * time.Second

// rehearsalKillGroup sends sig to process group pgid; sig 0 only probes
// it. A var only so a test can see, fake or drop what stop sends.
var rehearsalKillGroup = func(pgid int, sig syscall.Signal) error { return syscall.Kill(-pgid, sig) }

// rehearsalFakeawsTries is how many ports start tries. fakeaws cannot
// report an ephemeral port, so start picks a free one, releases it and
// starts fakeaws there; another process can take it in between.
const rehearsalFakeawsTries = 3

// rehearsalFakeawsPoll is how often start asks /healthz while it waits.
const rehearsalFakeawsPoll = 50 * time.Millisecond

// rehearsalFakeawsTail is how many bytes of fakeaws.log a refusal quotes.
const rehearsalFakeawsTail = 2048

// fakeawsAdminTokenEnv is where fakeaws reads its admin token from when no
// -admin-token flag is given.
const fakeawsAdminTokenEnv = "FAKEAWS_ADMIN_TOKEN"

// fakeawsBindFailure is what fakeaws logs when it exits because its port
// is taken: ListenAndServe's EADDRINUSE, through its log.Fatalf.
const fakeawsBindFailure = "address already in use"

var (
	errRehearsalFakeawsExited = errors.New("fakeaws exited before it was ready")
	errRehearsalFakeawsBind   = errors.New("fakeaws could not bind its port")
)

type rehearsalFakeawsOpts struct {
	bin, dir, account, region string
}

// rehearsalFakeaws is a started fakeaws: its pid, which is also its
// process group id, its loopback port and its admin token.
type rehearsalFakeaws struct {
	PID   int
	Port  int
	Token string
	// exited closes once the process has exited and been reaped.
	exited <-chan struct{}
	// gone is set once stop has seen the group gone. A later stop, on this
	// value or a copy, then sends nothing: an empty group's id is free for
	// the system to reuse.
	gone *atomic.Bool
}

// rehearsalStartFakeaws starts the rehearsal's fakeaws in its own process
// group and returns once it is ready and proved to be ours (ADR-0042
// § Start). It is started with exec.Command, never CommandContext, so it
// outlives ctx: ctx is watched only while start waits. A child that could
// not bind is retried on a new port; any other failure stops it and
// refuses.
func rehearsalStartFakeaws(ctx context.Context, opts rehearsalFakeawsOpts) (rehearsalFakeaws, error) {
	if err := os.Mkdir(opts.dir, 0o700); err != nil {
		return rehearsalFakeaws{}, fmt.Errorf("fakeaws cannot have its dir: %w", err)
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw) // crypto/rand.Read never returns an error: it crashes instead
	token := hex.EncodeToString(raw)
	var err error
	for range rehearsalFakeawsTries {
		var f rehearsalFakeaws
		if f, err = startRehearsalFakeawsOnce(ctx, opts, token); !errors.Is(err, errRehearsalFakeawsBind) {
			return f, err
		}
	}
	return rehearsalFakeaws{}, fmt.Errorf("%d tries in all: %w", rehearsalFakeawsTries, err)
}

// startRehearsalFakeawsOnce is one try on one free port. It returns
// errRehearsalFakeawsBind only for a child that exited because the port
// was taken.
func startRehearsalFakeawsOnce(ctx context.Context, opts rehearsalFakeawsOpts, token string) (rehearsalFakeaws, error) {
	port, err := rehearsalFreePort()
	if err != nil {
		return rehearsalFakeaws{}, fmt.Errorf("no free loopback port for fakeaws: %w", err)
	}
	logPath := filepath.Join(opts.dir, "fakeaws.log")
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return rehearsalFakeaws{}, fmt.Errorf("fakeaws cannot have its log: %w", err)
	}
	cmd := exec.Command(opts.bin,
		"-bind", "127.0.0.1", "-port", strconv.Itoa(port), "-db", filepath.Join(opts.dir, "fakeaws.db"),
		"-record-requests", "-account", opts.account, "-region", opts.region)
	// The token goes only here, never on argv, where ps shows it.
	cmd.Env = []string{fakeawsAdminTokenEnv + "=" + token}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// A nil Stdin is /dev/null.
	cmd.Stdout, cmd.Stderr = logFile, logFile
	err = cmd.Start()
	logFile.Close()
	if err != nil {
		return rehearsalFakeaws{}, fmt.Errorf("fakeaws did not start: %w", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	f := rehearsalFakeaws{PID: cmd.Process.Pid, Port: port, Token: token, exited: exited, gone: new(atomic.Bool)}

	err = f.waitReady(ctx)
	if err == nil {
		return f, nil
	}
	// A child failing on its own is judged by its exit and its log, whatever
	// the HTTP checks saw: while it fails to bind its port, another server
	// there can answer them. A cancel is not waited on.
	selfExited := false
	if ctx.Err() == nil {
		select {
		case <-f.exited:
			selfExited = true
		case <-time.After(rehearsalFakeawsExitGrace):
		}
	}
	stopErr := f.stop()
	tail := rehearsalFakeawsLogTail(logPath, token)
	switch {
	case stopErr != nil:
		return rehearsalFakeaws{}, fmt.Errorf("%w; %w; fakeaws.log ends:\n%s", err, stopErr, tail)
	case ctx.Err() != nil:
		return rehearsalFakeaws{}, fmt.Errorf("%w; fakeaws.log ends:\n%s", rehearsalCancelled(ctx), tail)
	case strings.Contains(tail, fakeawsBindFailure):
		return rehearsalFakeaws{}, fmt.Errorf("%w %d; fakeaws.log ends:\n%s", errRehearsalFakeawsBind, port, tail)
	case selfExited:
		return rehearsalFakeaws{}, fmt.Errorf("%w (%s); fakeaws.log ends:\n%s", errRehearsalFakeawsExited, cmd.ProcessState, tail)
	}
	return rehearsalFakeaws{}, fmt.Errorf("%w; fakeaws.log ends:\n%s", err, tail)
}

// stop ends fakeaws's process group: SIGTERM, then SIGKILL if any process
// is left in it once the fallback passes. It fails, naming pid and port,
// if the group is still there a fallback after the SIGKILL. It sends
// nothing to a group already gone, and nothing at all once a stop has
// seen it gone. A zero rehearsalFakeaws stops nothing: kill(-0) would
// signal the caller's own process group.
func (f rehearsalFakeaws) stop() error {
	if f.PID <= 0 || f.gone != nil && f.gone.Load() || f.groupGoneWithin(0) {
		return nil
	}
	_ = rehearsalKillGroup(f.PID, syscall.SIGTERM)
	if f.groupGoneWithin(rehearsalFakeawsKillFallback) {
		return nil
	}
	_ = rehearsalKillGroup(f.PID, syscall.SIGKILL)
	if f.groupGoneWithin(rehearsalFakeawsKillFallback) {
		return nil
	}
	return fmt.Errorf("stop failed: pid %d port %d", f.PID, f.Port)
}

// groupGoneWithin probes the group until no process is left in it, at
// most d (once, when d is 0), and records it gone. The group id cannot be
// reused while any process is left, so a probe that finds one reaches
// only this group. The leader counts until the goroutine waiting on it
// has reaped it.
func (f rehearsalFakeaws) groupGoneWithin(d time.Duration) bool {
	for deadline := time.Now().Add(d); ; time.Sleep(rehearsalFakeawsPoll) {
		if errors.Is(rehearsalKillGroup(f.PID, 0), syscall.ESRCH) {
			if f.gone != nil {
				f.gone.Store(true)
			}
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
	}
}

// waitReady waits, until rehearsalFakeawsReady or a cancel of ctx, for
// /healthz to answer 200, then checks once that the server is ours.
func (f rehearsalFakeaws) waitReady(ctx context.Context) error {
	waitCtx, cancel := context.WithTimeout(ctx, rehearsalFakeawsReady)
	defer cancel()
	// No proxy, and a redirect is an answer, never followed.
	client := &http.Client{
		Transport:     &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	base := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(f.Port))
	err := f.healthy(waitCtx, client, base)
	if err == nil {
		err = f.ours(waitCtx, client, base)
	}
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return rehearsalCancelled(ctx)
	case waitCtx.Err() != nil:
		return fmt.Errorf("fakeaws was not ready within %s: %w", rehearsalFakeawsReady, err)
	}
	return err
}

// healthy polls /healthz until it answers 200, the child exits, or ctx
// ends. It then returns the last answer, not the request ctx cut short.
func (f rehearsalFakeaws) healthy(ctx context.Context, client *http.Client, base string) error {
	tick := time.NewTicker(rehearsalFakeawsPoll)
	defer tick.Stop()
	last := errors.New("/healthz never answered")
	for {
		code, err := rehearsalFakeawsStatus(ctx, client, base+"/healthz", "")
		switch {
		case code == http.StatusOK:
			return nil
		case ctx.Err() != nil:
		case err != nil:
			last = err
		default:
			last = fmt.Errorf("/healthz answered %d", code)
		}
		select {
		case <-f.exited:
			return errRehearsalFakeawsExited
		case <-ctx.Done():
			return last
		case <-tick.C:
		}
	}
}

// ours proves the server on the port is this fakeaws and reachable only
// from this host: GET /mock/requests answers 200 with the token and 401
// without, and nothing answers on any of the host's non-loopback
// addresses.
func (f rehearsalFakeaws) ours(ctx context.Context, client *http.Client, base string) error {
	for _, check := range []struct {
		token, with string
		want        int
	}{
		{f.Token, "with its token", http.StatusOK},
		{"", "without its token", http.StatusUnauthorized},
	} {
		code, err := rehearsalFakeawsStatus(ctx, client, base+"/mock/requests", check.token)
		if err != nil {
			return fmt.Errorf("fakeaws is refused: GET /mock/requests %s failed: %w", check.with, err)
		}
		if code != check.want {
			return fmt.Errorf("fakeaws is refused: GET /mock/requests %s answered %d, not %d", check.with, code, check.want)
		}
	}
	ips, err := rehearsalNonLoopbackIPs()
	if err != nil {
		return fmt.Errorf("fakeaws is refused: this host's addresses cannot be listed: %w", err)
	}
	for _, ip := range ips {
		if err := rehearsalUnanswered(ctx, net.JoinHostPort(ip.String(), strconv.Itoa(f.Port))); err != nil {
			return err
		}
	}
	return nil
}

// rehearsalFakeawsStatus is the status of a GET of url, with the token as
// a bearer when it is not empty.
func rehearsalFakeawsStatus(ctx context.Context, client *http.Client, url, token string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// rehearsalNonLoopbackIPs is every address of this host that another host
// could dial: its global unicast addresses, private ones included.
func rehearsalNonLoopbackIPs() ([]net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var ips []net.IP
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.IsGlobalUnicast() {
			ips = append(ips, ipNet.IP)
		}
	}
	return ips, nil
}

// rehearsalUnanswered passes only when a dial of addr is refused or times
// out; a connection, or any other dial error, refuses.
func rehearsalUnanswered(ctx context.Context, addr string) error {
	dialCtx, cancel := context.WithTimeout(ctx, rehearsalDialTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	switch {
	case err == nil:
		conn.Close()
		return fmt.Errorf("fakeaws is refused: it answers on %s, not only on loopback", addr)
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(dialCtx.Err(), context.DeadlineExceeded):
		return nil
	}
	return fmt.Errorf("fakeaws is refused: a dial of %s neither connected nor was refused: %w", addr, err)
}

// rehearsalFreePort is a loopback port free a moment ago. A var only so a
// test can hand out a port another server holds.
var rehearsalFreePort = func() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// rehearsalFakeawsLogTail is the end of fakeaws.log, for a refusal. The
// token is never printed, so a log holding it anywhere is withheld whole,
// never redacted: the refusal names the log instead.
func rehearsalFakeawsLogTail(path, token string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(fakeaws.log cannot be read: %v)", err)
	}
	if strings.Contains(string(b), token) {
		return fmt.Sprintf("(withheld: it holds the admin token; read %s)", path)
	}
	if len(b) > rehearsalFakeawsTail {
		b = b[len(b)-rehearsalFakeawsTail:]
	}
	return strings.TrimSpace(string(b))
}
