#!/usr/bin/env python3
"""Runs a command in its own session under a deadline.

Usage: scenario_gate_deadline.py <deadline-seconds> <grace-seconds> <command> [args...]

At the deadline, on INT or TERM to this process, or once its parent is gone,
the command alone gets TERM: infrafactory then interrupts each tofu group once
itself, which a TERM to the whole tree would do twice. After <grace> seconds
more the command's process group gets KILL. On Linux, every process left in
the command's session gets KILL too, since tofu runs in its own process group
and survives infrafactory's exit. That sweep runs while the command is still
this process's unreaped zombie, so its pid, the session id, cannot have been
recycled, and nothing outside the session is hit. macOS cannot list a
session's members (its pkill has no -s), so there a tofu left behind relies on
infrafactory's own kill fallback.

Exits 124 when the deadline passed, else with the command's own status.
"""
import os
import signal
import subprocess
import sys
import time

POLL_SECONDS = 0.2
SWEEP_SECONDS = 10
EXIT_TIMED_OUT = 124


def log(msg):
    sys.stderr.write(f"scenario-gate-deadline: {msg}\n")
    sys.stderr.flush()


def sweep(sid):
    """KILLs every process left in session sid, the zombie sid itself aside. Linux only."""
    if not sys.platform.startswith("linux"):
        return
    left = subprocess.run(["pgrep", "-s", str(sid)], capture_output=True, text=True, timeout=SWEEP_SECONDS)
    pids = [p for p in left.stdout.split() if p != str(sid)]
    if not pids:
        return
    subprocess.run(["pkill", "-KILL", "-s", str(sid)], timeout=SWEEP_SECONDS)
    log(f"KILLed {len(pids)} process(es) left in the run's session")


def main():
    deadline, grace, cmd = float(sys.argv[1]), float(sys.argv[2]), sys.argv[3:]
    exited, stop = [], []
    signal.signal(signal.SIGCHLD, lambda *_: exited.append(True))
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stop.append(True))
    parent = os.getppid()
    pid = os.fork()
    if pid == 0:
        try:
            os.setsid()
            os.execvp(cmd[0], cmd)
        except OSError as err:
            log(f"cannot run {cmd[0]}: {err}")
            os._exit(127)
    start = time.monotonic()
    termed = killed = None
    timed_out = False
    while not exited:
        now = time.monotonic()
        if termed is None and (stop or now - start >= deadline or os.getppid() != parent):
            timed_out = now - start >= deadline
            log("TERM: " + ("deadline" if timed_out else "stopped" if stop else "parent gone"))
            os.kill(pid, signal.SIGTERM)
            termed = now
        elif termed is not None and killed is None and now - termed >= grace:
            log(f"KILL: still running {grace:g}s after TERM")
            os.killpg(pid, signal.SIGKILL)
            killed = now
        time.sleep(POLL_SECONDS)
    # The command is a zombie here: unreaped, so its pid, the session id, is still ours.
    sweep(pid)
    _, status = os.waitpid(pid, 0)
    if timed_out:
        sys.exit(EXIT_TIMED_OUT)
    sys.exit(os.WEXITSTATUS(status) if os.WIFEXITED(status) else 128 + os.WTERMSIG(status))


if __name__ == "__main__":
    main()
