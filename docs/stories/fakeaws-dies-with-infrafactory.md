---
kind: code
status: ready
depends_on: []
touches: [internal/cli/rehearse_fakeaws.go, internal/cli/rehearse_fakeaws_test.go]
---

# A rehearsal's fakeaws does not outlive infrafactory

Found in review of #623. internal/cli/rehearse_fakeaws.go:121 starts fakeaws with
`Setpgid: true` and no death signal, so a SIGKILLed infrafactory leaves fakeaws running and
holding its port. Use `childSysProcAttr()` from exec_runner_linux.go (it already sets Setpgid and,
on Linux, `Pdeathsig: SIGKILL`), and hold the OS thread across Start the way exec_runner.go Run
does. tofu's provider plugins get no death signal either (Pdeathsig reaches tofu alone); if there
is no small fix for them, say so in a `ponytail:` comment beside childSysProcAttr.

**Done when:**
- A Linux test starts fakeaws through the rehearsal path from a helper process, SIGKILLs the helper, and finds fakeaws gone within 2 seconds.
- macOS builds still pass.
