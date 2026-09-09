#!/usr/bin/env bash
# wait_for.sh — poll a health URL until it answers, or FAIL SAYING WHY.
#
# Usage: wait_for.sh <url> <name> [logfile] [timeout-seconds]
#
# Every bring-up target used to end in:
#
#     until curl -sSf "$URL" >/dev/null 2>&1; do sleep 1; done
#
# which is fine when the service starts and a silent trap when it does
# not. There is no bound, so a service that will never come up hangs the
# target forever: no error, no output, nothing on screen to say which of
# the six it was waiting for. `make up` just stops.
#
# Found on 2026-09-07 with Docker Desktop not running: `docker run` fails,
# SeaweedFS never listens, and the terminal is simply dead. SeaweedFS is
# only needed for AWS S3 scenarios, so the whole stack was blocked on a
# service most runs never touch.
#
# The Layer 3 arc already learned this in the CLI: a guard that stops
# without saying why is half a guard. Same rule here.
set -euo pipefail

url=${1:?usage: wait_for.sh <url> <name> [logfile] [timeout]}
name=${2:?usage: wait_for.sh <url> <name> [logfile] [timeout]}
logfile=${3:-}
timeout=${4:-60}

deadline=$(( $(date +%s) + timeout ))

# `tcp://host:port` waits for the LISTENER, not a health endpoint.
#
# Not every service has one. s3router logs "listening" and serves no
# /healthz at all, so the original loop for it fell back to `nc -z`.
# Replacing that with an HTTP-only wait turned a working bring-up into a
# 30-second failure -- the fallback was load-bearing and looked like
# noise.
probe() {
  case "$url" in
    tcp://*)
      hostport=${url#tcp://}
      nc -z "${hostport%%:*}" "${hostport##*:}" 2>/dev/null
      ;;
    *)
      curl -sSf "$url" >/dev/null 2>&1
      ;;
  esac
}

while true; do
  if probe; then
    exit 0
  fi

  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "ERROR: $name did not answer $url within ${timeout}s." >&2

    # The log is the whole point of failing loudly: it is the only place
    # the real cause lives -- a port already bound, a build error, a
    # daemon that is not running.
    if [ -n "$logfile" ] && [ -s "$logfile" ]; then
      echo "--- last 15 lines of $logfile ---" >&2
      tail -15 "$logfile" >&2
      echo "--- end ---" >&2
    elif [ -n "$logfile" ]; then
      echo "  ($logfile is empty or missing — the process may not have started at all)" >&2
    fi

    # Named separately because it is the most common cause and the least
    # obvious from a log that does not exist.
    case "$name" in
      seaweedfs)
        echo "  seaweedfs runs in Docker. Is Docker Desktop running?" >&2
        echo "  It is only needed for AWS S3 scenarios — 'make mockway-up' skips it." >&2
        ;;
    esac
    exit 1
  fi

  sleep 1
done
