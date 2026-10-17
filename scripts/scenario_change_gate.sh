#!/bin/bash
# M89 — scenario-change-gate. Detects added/modified scenarios in the
# current diff and runs each through `infrafactory run --clean`. Exits
# non-zero if any scenario diverges (status != success OR terminal
# reason != target_reached).
#
# Used by .github/workflows/scenario-gate.yml on PRs that touch
# scenarios/training/*.yaml. Also runnable locally:
#
#   BASE_REF=main bash scripts/scenario_change_gate.sh
#
# Env vars:
#   BASE_REF             — git ref to diff against (default: origin/main)
#   PER_SCENARIO_TIMEOUT — seconds per scenario before the run is TERMed
#                          (default: 720)
#   KILL_AFTER           — seconds after that TERM before the leftovers are
#                          KILLed; keep it above infrafactory's 20s tofu
#                          cancel fallback (default: 30)
#   MAX_SCENARIOS        — abort if more than N scenarios changed
#                          (default: 10 — protects against accidental
#                          mass-rename PRs)
#   RESULTS, LOG_DIR     — where the results TSV and logs go (see
#                          scenario_gate_paths.sh)
# A deadline or an INT/TERM to this script TERMs infrafactory only, lets its
# guard interrupt tofu, then KILLs what is left after KILL_AFTER. Needs pgrep
# (procps on Linux, built into macOS).
set -uo pipefail

BASE_REF="${BASE_REF:-origin/main}"
PER_SCENARIO_TIMEOUT="${PER_SCENARIO_TIMEOUT:-720}"
MAX_SCENARIOS="${MAX_SCENARIOS:-10}"
KILL_AFTER="${KILL_AFTER:-30}"
# shellcheck source=scripts/scenario_gate_paths.sh
. "$(dirname "$0")/scenario_gate_paths.sh"
mkdir -p "$LOG_DIR"

# descendants <pid>: prints the pid and every process below it, so a live run's
# tofu children (own process group, parent infrafactory) are found too.
descendants() {
  local child
  echo "$1"
  for child in $(pgrep -P "$1"); do
    descendants "$child"
  done
}

# escalate <pid>: runs KILL_AFTER after the TERM that was sent to the run. It
# KILLs what is left: the live run's tree, then the run's process group, which
# still holds orphans it spawned after it exited. infrafactory's own guard gets
# the KILL_AFTER window first; its tofu cleanup takes ~20s.
escalate() {
  local tree
  sleep "$KILL_AFTER"
  if kill -0 "$1" 2>/dev/null; then
    read -r -d "" -a tree <<< "$(descendants "$1")"
    kill -KILL "${tree[@]}" 2>/dev/null
  fi
  kill -KILL -- "-$1" 2>/dev/null
}

# stop_watchdog <pid>: stops a watchdog that has no deadline to enforce, and
# the sleep it is in.
stop_watchdog() {
  pkill -KILL -P "$1" 2>/dev/null
  kill -KILL "$1" 2>/dev/null
}

# cancel_run <pid> <watchdog> <exit-code>: the gate was interrupted. Stops the
# deadline, TERMs the run, waits out the escalation, then exits.
cancel_run() {
  trap '' INT TERM
  stop_watchdog "$2"
  kill -TERM "$1" 2>/dev/null
  escalate "$1"
  exit "$3"
}

# run_scenario <file> <log> <marker>: runs one scenario under a deadline. At
# the deadline, if the run is still alive, it writes <marker>, TERMs the run and
# escalates. The run gets its own process group (set -m) so the escalation can
# reach it. Whatever ends the run, the caller reads the marker: it says timeout.
run_scenario() {
  local f="$1" log="$2" marker="$3" pid watchdog
  rm -f "$marker"
  set -m
  ./bin/infrafactory run --clean "$f" > "$log" 2>&1 &
  pid=$!
  set +m
  (
    sleep "$PER_SCENARIO_TIMEOUT"
    kill -0 "$pid" 2>/dev/null || exit 0
    : > "$marker"
    kill -TERM "$pid" 2>/dev/null
    escalate "$pid"
  ) &
  watchdog=$!
  trap 'cancel_run "$pid" "$watchdog" 130' INT
  trap 'cancel_run "$pid" "$watchdog" 143' TERM
  wait "$pid"
  trap - INT TERM
  if [ -f "$marker" ]; then
    # The deadline fired: the escalation must finish even though the run is gone.
    wait "$watchdog"
    return
  fi
  stop_watchdog "$watchdog"
  wait "$watchdog" 2>/dev/null
}

# Discover changed scenarios. Diff filter AM = added + modified; we
# don't run deleted/renamed-source scenarios because the file is gone.
# Use a portable while-read pattern instead of mapfile (bash 4+) so
# the script runs on macOS's stock /bin/bash 3.2 as well as Ubuntu.
CHANGED_FILES=$(git diff --name-only --diff-filter=AM "$BASE_REF"...HEAD -- 'scenarios/training/*.yaml' 2>/dev/null)
CHANGED_COUNT=0
[ -n "$CHANGED_FILES" ] && CHANGED_COUNT=$(printf '%s\n' "$CHANGED_FILES" | wc -l | tr -d ' ')

if [ "$CHANGED_COUNT" -eq 0 ]; then
  echo "scenario-gate: no scenario changes in this PR — skipping"
  exit 0
fi

if [ "$CHANGED_COUNT" -gt "$MAX_SCENARIOS" ]; then
  echo "scenario-gate: ABORT — $CHANGED_COUNT scenarios changed (cap: $MAX_SCENARIOS)"
  echo "If this is intentional, override via MAX_SCENARIOS=$((CHANGED_COUNT+1))."
  exit 2
fi

echo "scenario-gate: running $CHANGED_COUNT changed scenario(s)"
printf '%s\n' "$CHANGED_FILES" | sed 's/^/  - /'
echo

# Build infrafactory fresh — the M93 lesson was that stale bin/infrafactory
# can silently invalidate sweep results.
echo "scenario-gate: building bin/infrafactory"
go build -o bin/infrafactory ./cmd/infrafactory
echo

: > "$RESULTS"
echo -e "scenario\tstatus\titers\tterminal_reason\tseconds" >> "$RESULTS"

overall_rc=0
while IFS= read -r f; do
  [ -z "$f" ] && continue
  name=$(basename "$f" .yaml)
  echo "[$(date +%H:%M:%S)] starting $name"
  start=$(date +%s)
  log="$LOG_DIR/$name.log"
  marker="$LOG_DIR/$name.deadline"
  run_scenario "$f" "$log" "$marker"
  end=$(date +%s)
  elapsed=$((end - start))

  status=$(grep -E "^Status:" "$log" | tail -1 | awk -F': ' '{print $2}' || echo "no-status")
  terminal=$(grep -E "^- run/terminal_reason:" "$log" | tail -1 | sed -E 's/.*pass \((.*)\)/\1/' || echo "no-terminal")
  iters=$(grep -cE "^- run/iteration_[0-9]+_generate:" "$log" || true)
  if [ -f "$marker" ]; then status="timeout"; terminal="timeout_${PER_SCENARIO_TIMEOUT}s"; fi

  echo -e "${name}\t${status}\t${iters}\t${terminal}\t${elapsed}" >> "$RESULTS"
  if [ "$status" = "success" ] && [ "$terminal" = "target_reached" ]; then
    echo "[$(date +%H:%M:%S)] $name: ✓ ($iters iter, ${elapsed}s)"
  else
    echo "[$(date +%H:%M:%S)] $name: ✗ $status / $terminal ($iters iter, ${elapsed}s)"
    overall_rc=1
  fi
done <<EOF
$CHANGED_FILES
EOF

echo
echo "=== scenario-gate summary ==="
column -t -s $'\t' "$RESULTS"

if [ "$overall_rc" -ne 0 ]; then
  echo
  echo "scenario-gate FAILED — at least one changed scenario did not converge."
  echo "Per-scenario logs: $LOG_DIR/"
  echo "Inspect the log for the failed scenario and either fix the YAML or"
  echo "add a pitfall via the M86+M90+M91+M92 auto-learning loop."
fi
exit "$overall_rc"
