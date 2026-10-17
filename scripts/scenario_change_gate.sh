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
#   PER_SCENARIO_TIMEOUT — seconds per scenario before its process tree is
#                          signalled (default: 720)
#   KILL_AFTER           — seconds between that TERM and a KILL (default: 30)
#   MAX_SCENARIOS        — abort if more than N scenarios changed
#                          (default: 10 — protects against accidental
#                          mass-rename PRs)
#   RESULTS, LOG_DIR     — where the results TSV and logs go (see
#                          scenario_gate_paths.sh)
# Needs pgrep (procps on Linux, built into macOS).
set -uo pipefail

BASE_REF="${BASE_REF:-origin/main}"
PER_SCENARIO_TIMEOUT="${PER_SCENARIO_TIMEOUT:-720}"
MAX_SCENARIOS="${MAX_SCENARIOS:-10}"
KILL_AFTER="${KILL_AFTER:-30}"
# shellcheck source=scripts/scenario_gate_paths.sh
. "$(dirname "$0")/scenario_gate_paths.sh"
mkdir -p "$LOG_DIR"

# descendants <pid>: prints the pid and every process below it. A tofu child
# sets its own process group, but its parent is still infrafactory, so this
# walk finds it.
descendants() {
  local child
  echo "$1"
  for child in $(pgrep -P "$1"); do
    descendants "$child"
  done
}

# run_scenario <file> <log> <marker>: runs one scenario under the deadline.
# When the deadline passes it writes <marker> and signals the run's whole
# process tree: TERM, then KILL after KILL_AFTER. Whichever signal ends the run,
# the caller reads the marker, so the row says timeout.
run_scenario() {
  local f="$1" log="$2" marker="$3" pid watchdog tree
  rm -f "$marker"
  ./bin/infrafactory run --clean "$f" > "$log" 2>&1 &
  pid=$!
  (
    sleep "$PER_SCENARIO_TIMEOUT"
    : > "$marker"
    read -r -d "" -a tree <<< "$(descendants "$pid")"
    kill -TERM "${tree[@]}" 2>/dev/null
    sleep "$KILL_AFTER"
    kill -KILL "${tree[@]}" 2>/dev/null
  ) &
  watchdog=$!
  trap 'kill -TERM $(descendants "$pid") 2>/dev/null; exit 130' INT
  wait "$pid"
  trap - INT
  read -r -d "" -a tree <<< "$(descendants "$watchdog")"
  kill -KILL "${tree[@]}" 2>/dev/null
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
