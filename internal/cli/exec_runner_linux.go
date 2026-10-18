package cli

import "syscall"

// childSysProcAttr puts the child in its own process group (see Run) and
// has the kernel SIGKILL it when infrafactory dies, however it dies: a
// SIGKILL or a cancelled CI job signals nothing, and tofu would run on.
// The death signal reaches tofu alone, not the provider plugins it
// started. It fires when the thread that started the child exits, so Run
// holds its OS thread until the child is reaped.
func childSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
