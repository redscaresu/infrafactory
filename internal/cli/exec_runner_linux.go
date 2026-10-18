package cli

import "syscall"

// childSysProcAttr puts the child in its own process group (see Run) and
// has the kernel SIGKILL it whenever infrafactory ends, a normal exit
// included: a SIGKILL or a cancelled CI job signals nothing, and tofu
// would run on with no parent.
//
// The trade-off is the kill Cancel's "interrupt, not kill" avoids. If
// infrafactory exits, or is killed, while tofu is still saving state
// after Cancel's SIGINT (inside cancelKillFallback), tofu now dies
// mid-flush where before it finished. A tofu left applying with no parent
// is the worse outcome, so this takes the kill.
//
// The death signal reaches tofu alone, not the provider plugins it
// started. It fires when the thread that started the child exits, so Run
// holds its OS thread until the child is reaped.
func childSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
