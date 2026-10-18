#!/usr/bin/env python3
"""Runs a command in its own session under a deadline.

Usage: scenario_gate_deadline.py <deadline-seconds> <grace-seconds> <command> [args...]

At the deadline, on INT or TERM to this process, or once its parent is gone,
the command alone gets TERM: infrafactory then interrupts each tofu group once
itself, which a TERM to the whole tree would do twice. After <grace> seconds
more the command's process group gets KILL, which also ends a stopped command
that a TERM could not. On Linux, every process left in the command's session
gets KILL too, since tofu runs in its own process group and survives
infrafactory's exit. That sweep runs while the command is still this process's
unreaped zombie, so its pid, the session id, cannot have been recycled, and
nothing outside the session is hit. macOS cannot list a session's members (its
pkill has no -s), so there a tofu left behind relies on infrafactory's own kill
fallback; it has no os.waitid either, so there the command is reaped as soon
as it exits.

A signal or sweep that fails is logged and skipped: the command is reaped
either way, and a passed deadline still exits 124. Otherwise the exit status
is the command's own.
"""
import os
import signal
import subprocess
import sys
import time

POLL_SECONDS = 0.2
SWEEP_SECONDS = 10
EXIT_TIMED_OUT = 124
EXIT_CANNOT_RUN = 127


def log(msg):
    sys.stderr.write(f"scenario-gate-deadline: {msg}\n")
    sys.stderr.flush()


def send(what, kill, *args):
    """kill(*args), logging what it was for; a child already gone is not an error."""
    try:
        kill(*args)
    except (ProcessLookupError, PermissionError) as err:
        log(f"{what}: {err}")


def sweep(sid):
    """KILLs every process left in session sid, the zombie sid itself aside. Linux only."""
    if not sys.platform.startswith("linux"):
        return
    try:
        left = subprocess.run(["pgrep", "-s", str(sid)], capture_output=True, text=True, timeout=SWEEP_SECONDS)
        pids = [p for p in left.stdout.split() if p != str(sid)]
        if not pids:
            return
        subprocess.run(["pkill", "-KILL", "-s", str(sid)], timeout=SWEEP_SECONDS)
    except (subprocess.TimeoutExpired, OSError) as err:
        log(f"sweep of the run's session failed: {err}")
        return
    log(f"KILLed {len(pids)} process(es) left in the run's session")


def exited(pid, reaped):
    """True once the child has exited; a stopped child has not. With os.waitid
    (Linux) the child stays an unreaped zombie for the sweep. Without it (macOS)
    the child is reaped here and its wait status appended to reaped."""
    if hasattr(os, "waitid"):
        return os.waitid(os.P_PID, pid, os.WEXITED | os.WNOHANG | os.WNOWAIT) is not None
    wpid, status = os.waitpid(pid, os.WNOHANG)
    if wpid:
        reaped.append(status)
    return bool(wpid)


def main():
    deadline, grace, cmd = float(sys.argv[1]), float(sys.argv[2]), sys.argv[3:]
    stop = []
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stop.append(True))
    parent = os.getppid()
    try:
        # start_new_session is setsid() in the child. Popen also gives the child
        # the default SIGPIPE and SIGXFSZ dispositions that Python itself ignores.
        child = subprocess.Popen(cmd, start_new_session=True)
    except OSError as err:
        log(f"cannot run {cmd[0]}: {err}")
        sys.exit(EXIT_CANNOT_RUN)
    pid = child.pid
    start = time.monotonic()
    termed = killed = None
    timed_out = False
    reaped = []
    while not exited(pid, reaped):
        now = time.monotonic()
        if termed is None and (stop or now - start >= deadline or os.getppid() != parent):
            timed_out = now - start >= deadline
            log("TERM: " + ("deadline" if timed_out else "stopped" if stop else "parent gone"))
            send("TERM", os.kill, pid, signal.SIGTERM)
            termed = now
        elif termed is not None and killed is None and now - termed >= grace:
            log(f"KILL: still running {grace:g}s after TERM")
            send("KILL", os.killpg, pid, signal.SIGKILL)
            killed = now
        time.sleep(POLL_SECONDS)
    # On Linux the command is a zombie here: unreaped, so its pid, the session id, is still ours.
    sweep(pid)
    status = reaped[0] if reaped else os.waitpid(pid, 0)[1]
    child.returncode = 0  # reaped above, so Popen's own cleanup has nothing left to wait for
    if timed_out:
        sys.exit(EXIT_TIMED_OUT)
    sys.exit(os.WEXITSTATUS(status) if os.WIFEXITED(status) else 128 + os.WTERMSIG(status))


if __name__ == "__main__":
    main()
