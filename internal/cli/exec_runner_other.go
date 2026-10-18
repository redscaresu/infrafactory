//go:build !linux

package cli

import "syscall"

// childSysProcAttr puts the child in its own process group (see Run).
//
// ponytail: macOS has no Pdeathsig, so a tofu whose infrafactory is
// SIGKILLed runs on with no parent. The fix is a watchdog process outside
// infrafactory that kills the group when a pipe from infrafactory closes;
// add it if a killed local run ever leaves tofu applying. CI runs on
// Linux, where exec_runner_linux.go closes the gap for tofu itself, not
// its provider plugins.
func childSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
