package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A SIGKILLed infrafactory signals nothing, so the kernel must end tofu:
// otherwise it runs on, applying, with no one to read its state.
func TestExecRunnerChildDiesWithInfrafactory(t *testing.T) {
	t.Parallel()

	ready := filepath.Join(t.TempDir(), "ready")
	helper := exec.Command(os.Args[0], "-test.run=^TestExecRunnerHelperProcess$")
	helper.Env = append(os.Environ(), execHelperEnv+"=orphan", execHelperReadyEnv+"="+ready)
	require.NoError(t, helper.Start())
	waitForFile(t, ready)
	raw, err := os.ReadFile(ready)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	require.True(t, processAlive(pid), "the sleep never started")

	require.NoError(t, helper.Process.Kill())
	_ = helper.Wait()

	assert.Eventually(t, func() bool { return !processAlive(pid) }, 2*time.Second, 10*time.Millisecond,
		"the sleep outlived the process that started it")
}

// processAlive reports whether pid runs. A zombie counts as dead: its
// reaper, not the sleep, decides when it goes.
func processAlive(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// The state follows the parenthesised command name, which may hold spaces.
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	return len(fields) > 0 && fields[0] != "Z"
}
