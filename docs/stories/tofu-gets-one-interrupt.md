---
kind: code
status: ready
depends_on: []
touches: ["internal/cli/exec_runner.go", "internal/cli/exec_runner_test.go"]
---

# One Ctrl-C reaches tofu once, so it stops gracefully and keeps its state

Seen on real AWS in aws-layer3-claim-legs (2026-10-10, run 20261010T103819Z). One SIGINT to the
foreground process group during the apply made tofu print "Two interrupts received. Exiting
immediately. Note that data loss may have occurred." The terminal signals the whole group, tofu
included, and then infrafactory's context cancel sends tofu a second SIGINT
(internal/cli/exec_runner.go, `execCmd.Cancel`). Its comment already names the gap: the
interactive case "needs Setpgid so the child is not in the terminal's group". Since #429 one
Ctrl-C is meant to finish the teardown, so a tofu that hard-exits mid-apply can lose state for
resources it was creating; that run's sweep caught them, but destroy should not need it to.

Run tofu in its own process group (`SysProcAttr.Setpgid`), so the terminal's SIGINT reaches only
infrafactory, and the cancel's SIGINT is tofu's first. Keep the SIGKILL fallback.

**Done when:**
- A test starts a child through the exec runner, sends SIGINT to the parent's process group, and
  shows the child receives exactly one SIGINT (a helper process that counts signals). It fails
  without Setpgid, shown by mutation.
- Non-interactive cancel (a cancelled context, a timeout) still sends SIGINT, then SIGKILL after
  `cancelKillFallback`, as today.
