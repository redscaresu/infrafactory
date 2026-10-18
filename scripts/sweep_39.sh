#!/usr/bin/env bash
# scripts/sweep_39.sh — canonical 39-scenario sustain-ratchet sweep.
#
# Drives `infrafactory run` across every scenario under
# scenarios/training/. Uses `infrafactory mock reset` between
# scenarios so the SeaweedFS cascade fires correctly (a bare
# `curl -X POST /mock/reset` to fakeaws does NOT cascade — see
# the S54 SeaweedFS state-leak post-mortem in
# docs/archive/status/ARCHIVE.md).
#
# Output:
#   $SWEEP_DIR/summary.tsv     — per-scenario terminal_reason / iter / dur
#   $SWEEP_DIR/<scenario>.log  — full run stdout/stderr
#   $SWEEP_DIR/<cloud>.pitfalls.diff
#
# SWEEP_RUN_TIMEOUT (seconds, default 1800) bounds each scenario's run. A run
# that hits it is recorded as timed_out and the sweep moves on to the next. A run
# that dies after its startup run.json (status "running") is recorded as crashed,
# and one that writes no run.json at all as no_result. SWEEP_RESET_TIMEOUT
# (seconds, default 300) bounds each `mock reset`. A reset that hits it stops the
# loop: the blocked scenario's row is recorded as reset_timed_out, or, on the S101
# retry, the first attempt's row is kept. The tail then runs as usual, and the
# sweep exits 3 at its end. If the panic gate also finds a panic, the sweep
# exits 2 instead, since a panic is a real regression; the reset error is
# printed either way. SWEEP_KILL_GRACE (seconds, default 30) is the wait
# between TERM and KILL for both. MOCK_LOG_ROOT
# (default /private/tmp) is where the panic gate looks for mock logs.
#
# Discards pitfalls/<cloud>.yaml additions per
# `feedback_sweep_protocol.md` — they're sweep noise that re-emerges
# naturally on the next run.
#
# Replaces the inline /tmp/sweep-*.sh scripts every prior arc
# reinvented. S78 (slices-74-78-plan.md).
set -u

ROOT="${ROOT:-$(git rev-parse --show-toplevel)}"
SWEEP_DIR="${SWEEP_DIR:-/tmp/sweep-39}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SWEEP_RUN_TIMEOUT="${SWEEP_RUN_TIMEOUT:-1800}"
SWEEP_RESET_TIMEOUT="${SWEEP_RESET_TIMEOUT:-300}"
RESET_TIMED_OUT_EXIT=3
# The deadline script exits with this when the deadline passed.
DEADLINE_TIMED_OUT=124
# The grace must exceed infrafactory's cancelKillFallback (internal/cli/exec_runner.go).
# sweep_39_test.sh checks this default against that value.
SWEEP_KILL_GRACE="${SWEEP_KILL_GRACE:-30}"
DEADLINE="$SCRIPT_DIR/scenario_gate_deadline.py"
MOCK_LOG_ROOT="${MOCK_LOG_ROOT:-/private/tmp}"
RUN_MARKER="$SWEEP_DIR/.run-started"

# The deadline script takes whole seconds; anything else would fail every run
# before it started.
for v in SWEEP_RUN_TIMEOUT SWEEP_RESET_TIMEOUT SWEEP_KILL_GRACE; do
  if ! [[ "${!v}" =~ ^[1-9][0-9]*$ ]]; then
    echo "sweep: $v must be a positive whole number of seconds, got '${!v}'" >&2
    exit 1
  fi
done

mkdir -p "$SWEEP_DIR"
cd "$ROOT" || exit 1
unset CLAUDECODE

# The deadline script runs under python3. Without it, no scenario run or mock
# reset starts, and the rows would have nothing to report.
if ! command -v python3 >/dev/null; then
  echo "sweep: python3 is needed to run each scenario under SWEEP_RUN_TIMEOUT and each mock reset under SWEEP_RESET_TIMEOUT" >&2
  exit 1
fi
if [ ! -f "$DEADLINE" ]; then
  echo "sweep: deadline script not found at $DEADLINE" >&2
  exit 1
fi

if [ ! -x ./bin/infrafactory ]; then
  echo "==> bin/infrafactory not found; running make build"
  make build || exit 1
fi

SUMMARY="$SWEEP_DIR/summary.tsv"
echo -e "scenario\tterminal_reason\tstatus\titers\tdur_s" > "$SUMMARY"

PRE="$SWEEP_DIR/pre-pitfalls"
mkdir -p "$PRE"
cp pitfalls/aws.yaml "$PRE/aws.yaml"
cp pitfalls/gcp.yaml "$PRE/gcp.yaml"
cp pitfalls/scaleway.yaml "$PRE/scaleway.yaml"

if [ -n "${SCENARIOS_FILE:-}" ]; then
  cp "$SCENARIOS_FILE" "$SWEEP_DIR/scenarios.txt"
else
  ls scenarios/training/ | grep -v '^gcp-full-stack$' > "$SWEEP_DIR/scenarios.txt"
fi

RETRY_TRANSPORT_TOTAL=0
RETRY_RECOVERED_TOTAL=0
RESET_TIMED_OUT=0

# is_transport_failed_shape — S101 heuristic for transport-class
# failures. Returns 0 (true) if the failure matches the transport
# shape we expect to recover with a single retry. Two known classes:
#
#   1. Claude CLI rate-limit (S97): pre-iter-1 _generate stage fails
#      in 5-9s. terminal=repair_budget_exhausted.
#   2. OpenTofu provider-registry 502 (S100 sweep 2): _validate stage
#      `tofu init` fails fetching provider checksums in 50-60s.
#      terminal=stuck.
#
# Heuristic:
#   - terminal in {repair_budget_exhausted, stuck}
#   - dur_s < 60 (covers both classes; real LLM convergence sticks
#     for >120s typically)
#   - zero _test stage failures (a _test failure is LLM-side, not
#     transport)
#   - at least one _generate OR _validate stage failure
#
# False positives surface as `transport_failed` rows the operator
# can re-inspect. False negatives miss the retry opportunity but
# don't break anything.
is_transport_failed_shape() {
  local term="$1" duration="$2" logfile="$3"
  case "$term" in
    repair_budget_exhausted|stuck) ;;
    *) return 1 ;;
  esac
  [ "${duration:-99}" -lt 60 ] || return 1
  [ -f "$logfile" ] || return 1
  local test_fails generate_fails validate_fails
  test_fails=$(grep -cE '"stage_end".*"status":"failed".*"stage":"iteration_[0-9]+_test"' "$logfile" 2>/dev/null)
  if [ "${test_fails:-0}" != "0" ]; then return 1; fi
  generate_fails=$(grep -cE '"stage_end".*"status":"failed".*"stage":"iteration_[0-9]+_generate"' "$logfile" 2>/dev/null)
  validate_fails=$(grep -cE '"stage_end".*"status":"failed".*"stage":"iteration_[0-9]+_validate"' "$logfile" 2>/dev/null)
  if [ "${generate_fails:-0}" = "0" ] && [ "${validate_fails:-0}" = "0" ]; then
    return 1
  fi
  return 0
}

# Selective pitfall restoration. The blanket `git checkout pitfalls/`
# discarded everything — including `avoid` entries, which are grounded
# in confirmed deletion-as-fix runs (iter N failed, iter N+1 succeeded
# after removing a resource). Replace with a selective merge that keeps
# only `avoid` from the post-sweep file and discards `fix` + `descriptive`
# as sweep noise. Runs at the end of a sweep and when one is stopped.
restore_pitfalls() {
  echo
  echo "=== avoid-pitfall durability ==="
  if [ ! -x ./bin/pitfall-merge ]; then
    echo "WARN: bin/pitfall-merge not found; falling back to blanket discard"
    git checkout pitfalls/ 2>/dev/null || true
    avoid_total=0
    return
  fi
  avoid_total=0
  for c in aws gcp scaleway; do
    out=$(./bin/pitfall-merge \
      --pre "$PRE/$c.yaml" \
      --post "pitfalls/$c.yaml" \
      --out "pitfalls/$c.yaml" \
      --keep avoid,live 2>&1)
    echo "  $c: $out"
    # kept_avoid, not kept_new: kept_new now also counts preserved
    # `live` entries, and folding those in would let live preservation
    # mask an avoid-learning regression -- a ratchet that reads healthy
    # while the thing it measures is broken.
    added=$(echo "$out" | grep -oE 'kept_avoid=[0-9]+' | sed 's/kept_avoid=//')
    avoid_total=$((avoid_total + ${added:-0}))
  done
}

# Ctrl-C or TERM stops the sweep. The deadline script runs in the background,
# so the trap fires at once: it TERMs the scenario run or the mock reset in
# progress (the deadline script KILLs its group after the grace), the pitfalls
# are restored, and the sweep exits by the same signal. Otherwise each Ctrl-C
# would only end the scenario or reset in progress. Signals are ignored while
# restoring, so a second Ctrl-C cannot skip the restore.
watchdog=
stop_sweep() {
  trap '' INT TERM
  if [ -n "$watchdog" ]; then
    kill -TERM "$watchdog" 2>/dev/null
    wait "$watchdog" 2>/dev/null
  fi
  restore_pitfalls
  trap - INT TERM
  kill -"$1" $$
}
trap 'stop_sweep INT' INT
trap 'stop_sweep TERM' TERM

# run_bounded <deadline> <out> <command...>: runs the command under the deadline
# script in the background, so a TERM to the sweep reaches the stop trap at once
# rather than after the command ends. Sets BOUNDED_EC.
run_bounded() {
  local deadline="$1" out="$2"
  shift 2
  python3 "$DEADLINE" "$deadline" "$SWEEP_KILL_GRACE" "$@" > "$out" 2>&1 &
  watchdog=$!
  wait "$watchdog"
  BOUNDED_EC=$?
  watchdog=
}

# reset_mocks: `mock reset` under SWEEP_RESET_TIMEOUT. Sets RESET_TIMED_OUT when
# the reset misses its deadline. Other failures of the reset stay ignored, as before.
reset_mocks() {
  RESET_TIMED_OUT=0
  run_bounded "$SWEEP_RESET_TIMEOUT" /dev/null ./bin/infrafactory mock reset --config infrafactory.yaml
  if [ "$BOUNDED_EC" -eq "$DEADLINE_TIMED_OUT" ]; then
    RESET_TIMED_OUT=1
  fi
}

run_scenario() {
  local sc="$1" name="$2" log="$3"
  reset_mocks
  if [ "$RESET_TIMED_OUT" -eq 1 ]; then
    # No run wrote this log, so any left under the name is an earlier sweep's.
    rm -f "$log"
    RUN_TERMINAL=reset_timed_out
    RUN_STATUS=reset_timed_out
    RUN_ITERS=0
    RUN_DUR=0
    RUN_EC=$BOUNDED_EC
    return
  fi
  local start
  start=$(date +%s)
  touch "$RUN_MARKER"
  run_bounded "$SWEEP_RUN_TIMEOUT" "$log" ./bin/infrafactory run "scenarios/training/$sc" --config infrafactory.yaml
  RUN_EC=$BOUNDED_EC
  RUN_DUR=$(( $(date +%s) - start ))
  # grep -c already prints 0 on no match; `|| echo 0` would add a second 0.
  RUN_ITERS=$(grep -c '"event":"iteration_start"' "$log" 2>/dev/null)
  RUN_ITERS=${RUN_ITERS:-0}
  if [ "$RUN_EC" -eq "$DEADLINE_TIMED_OUT" ]; then
    RUN_TERMINAL=timed_out
    RUN_STATUS=timed_out
    return
  fi
  # Only a run.json written after RUN_MARKER came from this run. With none, the
  # run never got as far as its startup write: no_result. An older run.json
  # would report a run that is not this one.
  local rj
  rj=$(find ".infrafactory/runs/$name" -name run.json -newer "$RUN_MARKER" 2>/dev/null | sort | tail -1)
  if [ -z "$rj" ]; then
    RUN_TERMINAL=no_result
    RUN_STATUS=no_result
    return
  fi
  RUN_TERMINAL=$(grep -oE '"terminal_reason":[ ]*"[a-z_]+"' "$rj" 2>/dev/null | head -1 | sed 's/.*"\([a-z_]*\)"/\1/')
  RUN_STATUS=$(grep -oE '"status":[ ]*"[a-z_]+"' "$rj" 2>/dev/null | head -1 | sed 's/.*"\([a-z_]*\)"/\1/')
  # infrafactory writes status "running" at startup and the final status at
  # exit, so a run that died partway still reads "running".
  if [ "$RUN_STATUS" = running ]; then
    RUN_TERMINAL=crashed
    RUN_STATUS=crashed
  fi
}

while IFS= read -r sc; do
  name="${sc%.yaml}"
  log="$SWEEP_DIR/$name.log"
  run_scenario "$sc" "$name" "$log"
  # S101 single-shot retry. If the run hit a transport-failed shape,
  # retry ONCE. Replace the row with the retry's result. The retry's
  # outcome — pass, transport-fail again, or convergence-fail — is
  # the final row in summary.tsv. Single-shot, not a loop: if the
  # retry also fails, we don't keep retrying.
  if is_transport_failed_shape "$RUN_TERMINAL" "$RUN_DUR" "$log"; then
    echo "  RETRY $name (transport shape: terminal=$RUN_TERMINAL dur=${RUN_DUR}s)"
    RETRY_TRANSPORT_TOTAL=$((RETRY_TRANSPORT_TOTAL + 1))
    mv "$log" "$log.attempt1"
    first_terminal=$RUN_TERMINAL first_status=$RUN_STATUS first_iters=$RUN_ITERS first_dur=$RUN_DUR first_ec=$RUN_EC
    run_scenario "$sc" "$name" "$log"
    if [ "$RESET_TIMED_OUT" -eq 1 ]; then
      # The retry never ran, so the row is the attempt that did, with its own log.
      mv "$log.attempt1" "$log"
      RUN_TERMINAL=$first_terminal RUN_STATUS=$first_status RUN_ITERS=$first_iters RUN_DUR=$first_dur RUN_EC=$first_ec
    fi
    if [ "$RUN_TERMINAL" = "target_reached" ]; then
      RETRY_RECOVERED_TOTAL=$((RETRY_RECOVERED_TOTAL + 1))
      echo "  RECOVERED $name on retry (terminal=$RUN_TERMINAL dur=${RUN_DUR}s)"
    fi
  fi
  echo -e "$name\t${RUN_TERMINAL:-unknown}\t${RUN_STATUS:-unknown}\t$RUN_ITERS\t$RUN_DUR" >> "$SUMMARY"
  printf "%-32s %-22s %-9s iters=%-2s dur=%ss ec=%s\n" "$name" "${RUN_TERMINAL:-unknown}" "${RUN_STATUS:-unknown}" "$RUN_ITERS" "$RUN_DUR" "$RUN_EC"
  # The next scenario would run against mocks of unknown state, so the loop
  # stops here. The tail still runs, and exits 3 at its end.
  if [ "$RESET_TIMED_OUT" -eq 1 ]; then
    break
  fi
done < "$SWEEP_DIR/scenarios.txt"

echo
echo "=== retry summary ==="
echo "RETRY_TRANSPORT=$RETRY_TRANSPORT_TOTAL"
echo "RETRY_RECOVERED=$RETRY_RECOVERED_TOTAL"

# S97 (2026-06-03): reclassify pre-iter-1 LLM transport failures.
# Sustain sweep 3 produced 6 scenarios with shape:
#   $name  repair_budget_exhausted  failed  2  5
# Two iterations, 5-9s durations, both iterations failing at
# iteration_*_generate (the claude CLI invocation itself — never
# reached test or validate). That's a Claude rate-limit cluster, not
# LLM convergence. Without separating them out, "X/39" conflates
# deterministic flakes with transport blips.
#
# Heuristic: dur_s < 30 AND every iteration failed at the generate
# stage (no _test or _validate failures). Approximate — false
# positives surface as transport_failed rows the operator can
# re-inspect; false negatives stay as repair_budget_exhausted (the
# existing behaviour).
#
# Doesn't retry. That's bigger work (LLM-transport robustness arc).
# S97 just classifies.
echo
echo "=== transport classifier ==="
TRANSPORT_COUNT=0
NEW_SUMMARY="$SWEEP_DIR/summary.tsv.new"
: > "$NEW_SUMMARY"
while IFS=$'\t' read -r name terminal status iters dur; do
  if [ "$name" = "scenario" ]; then
    echo -e "$name\t$terminal\t$status\t$iters\t$dur" >> "$NEW_SUMMARY"
    continue
  fi
  # S101 (2026-06-04): use the same widened transport-shape predicate
  # the in-loop retry uses, so a row that still matches transport
  # after retry gets correctly labeled. Reuses is_transport_failed_shape.
  log="$SWEEP_DIR/$name.log"
  if is_transport_failed_shape "$terminal" "$dur" "$log"; then
    terminal="transport_failed"
    TRANSPORT_COUNT=$((TRANSPORT_COUNT + 1))
    echo "  $name reclassified -> transport_failed (dur=${dur}s, terminal-was=$(awk -F$'\t' -v n="$name" 'NR>1 && $1==n {print $2}' "$SUMMARY"))"
  fi
  echo -e "$name\t$terminal\t$status\t$iters\t$dur" >> "$NEW_SUMMARY"
done < "$SUMMARY"
mv "$NEW_SUMMARY" "$SUMMARY"
echo "TRANSPORT_FAILED=$TRANSPORT_COUNT"

echo
echo "=== summary ==="
column -t -s$'\t' < "$SUMMARY"
echo
for c in aws gcp scaleway; do
  diff -u "$PRE/$c.yaml" "pitfalls/$c.yaml" > "$SWEEP_DIR/$c.pitfalls.diff" 2>/dev/null || true
  added=$(grep -cE "^+.*source: (fix|avoid)" "$SWEEP_DIR/$c.pitfalls.diff" 2>/dev/null || echo 0)
  echo "$c: +$added diff-derived lines"
done

pass=$(awk -F$'\t' 'NR>1 && $2=="target_reached"' "$SUMMARY" | wc -l | tr -d ' ')
total=$(awk -F$'\t' 'NR>1' "$SUMMARY" | wc -l | tr -d ' ')
echo
transport=$(awk -F$'\t' 'NR>1 && $2=="transport_failed"' "$SUMMARY" | wc -l | tr -d ' ')
non_transport_total=$((total - transport))
echo "PASS=$pass / TOTAL=$total (deterministic: $pass/$non_transport_total; transport_failed: $transport)"

# S87 panic-detection gate. Mocks logs are not perfectly stable in
# layout across `make mocks-up` vs `make mocks-up-containers`, so
# the probe checks both. A panic line in any mock log fails the
# sweep exit code so CI surfaces the regression — the alternative
# was silent re-occurrence of the historical `plugin did not
# respond` class S86 found stale on 2026-06-03.
#
# Detects: `panic:`, `runtime error:`, `recovered from panic`,
# `nil pointer dereference`. Lower-cased grep so case variants
# don't slip through.
echo
echo "=== panic gate ==="
PANIC_LOG="$SWEEP_DIR/panics.log"
: > "$PANIC_LOG"
for log in "$MOCK_LOG_ROOT"/infrafactory-mocks/{fakegcp,fakeaws,mockway,s3router}.log \
           "$MOCK_LOG_ROOT"/{fakegcp,fakeaws,mockway}.log; do
  [ -f "$log" ] || continue
  if grep -iE 'panic:|runtime error:|recovered from panic|nil pointer dereference' "$log" >> "$PANIC_LOG" 2>/dev/null; then
    echo "PANIC in $log:"
    grep -iE 'panic:|runtime error:|recovered from panic|nil pointer dereference' "$log" | head -5 | sed 's/^/  /'
  fi
done

panic_lines=$(wc -l < "$PANIC_LOG" | tr -d ' ')
echo "PANIC_LINES=$panic_lines (summary at $PANIC_LOG)"

restore_pitfalls
echo "AVOID_EMISSIONS=$avoid_total"
if [ "$avoid_total" = "0" ]; then
  echo "WARN: zero avoid-pitfall emissions this sweep. Could be (a) the LLM stopped making deletion-recoverable mistakes, or (b) the avoid extractor broke. Cross-reference next sweep before treating as a regression."
fi

if [ "$RESET_TIMED_OUT" -eq 1 ]; then
  echo "sweep: mock reset did not return within ${SWEEP_RESET_TIMEOUT}s; the sweep stopped before the remaining scenarios" >&2
fi
# Exit non-zero if any panic surfaced — that's a real regression, so it wins
# over a reset stop.
if [ "$panic_lines" -gt 0 ]; then
  echo "FAIL: panic-gate detected $panic_lines line(s); see $PANIC_LOG"
  exit 2
fi
if [ "$RESET_TIMED_OUT" -eq 1 ]; then
  exit "$RESET_TIMED_OUT_EXIT"
fi
