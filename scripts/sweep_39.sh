#!/usr/bin/env bash
# scripts/sweep_39.sh — canonical 39-scenario sustain-ratchet sweep.
#
# Drives `infrafactory run` across every scenario under
# scenarios/training/. Uses `infrafactory mock reset` between
# scenarios so the SeaweedFS cascade fires correctly (a bare
# `curl -X POST /mock/reset` to fakeaws does NOT cascade — see
# the S54 SeaweedFS state-leak post-mortem in
# docs/status/ARCHIVE.md).
#
# Output:
#   $SWEEP_DIR/summary.tsv     — per-scenario terminal_reason / iter / dur
#   $SWEEP_DIR/<scenario>.log  — full run stdout/stderr
#   $SWEEP_DIR/<cloud>.pitfalls.diff
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
mkdir -p "$SWEEP_DIR"
cd "$ROOT" || exit 1
unset CLAUDECODE

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

ls scenarios/training/ | grep -v '^gcp-full-stack$' > "$SWEEP_DIR/scenarios.txt"

RETRY_TRANSPORT_TOTAL=0
RETRY_RECOVERED_TOTAL=0

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

run_scenario() {
  local sc="$1" name="$2" log="$3"
  ./bin/infrafactory mock reset --config infrafactory.yaml >/dev/null 2>&1 || true
  local start
  start=$(date +%s)
  ./bin/infrafactory run "scenarios/training/$sc" --config infrafactory.yaml > "$log" 2>&1
  RUN_EC=$?
  RUN_DUR=$(( $(date +%s) - start ))
  local rj
  rj=$(ls -td .infrafactory/runs/$name/*/ 2>/dev/null | head -1)run.json
  RUN_TERMINAL=$(grep -oE '"terminal_reason":[ ]*"[a-z_]+"' "$rj" 2>/dev/null | head -1 | sed 's/.*"\([a-z_]*\)"/\1/')
  RUN_STATUS=$(grep -oE '"status":[ ]*"[a-z_]+"' "$rj" 2>/dev/null | head -1 | sed 's/.*"\([a-z_]*\)"/\1/')
  RUN_ITERS=$(grep -c '"event":"iteration_start"' "$log" 2>/dev/null || echo 0)
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
    run_scenario "$sc" "$name" "$log"
    if [ "$RUN_TERMINAL" = "target_reached" ]; then
      RETRY_RECOVERED_TOTAL=$((RETRY_RECOVERED_TOTAL + 1))
      echo "  RECOVERED $name on retry (terminal=$RUN_TERMINAL dur=${RUN_DUR}s)"
    fi
  fi
  echo -e "$name\t${RUN_TERMINAL:-unknown}\t${RUN_STATUS:-unknown}\t$RUN_ITERS\t$RUN_DUR" >> "$SUMMARY"
  printf "%-32s %-22s %-9s iters=%-2s dur=%ss ec=%s\n" "$name" "${RUN_TERMINAL:-unknown}" "${RUN_STATUS:-unknown}" "$RUN_ITERS" "$RUN_DUR" "$RUN_EC"
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
  added=$(grep -c "^+.*learned_from_diff" "$SWEEP_DIR/$c.pitfalls.diff" 2>/dev/null || echo 0)
  echo "$c: +$added learned_from_diff* lines"
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
for log in /private/tmp/infrafactory-mocks/fakegcp.log \
           /private/tmp/infrafactory-mocks/fakeaws.log \
           /private/tmp/infrafactory-mocks/mockway.log \
           /private/tmp/infrafactory-mocks/s3router.log \
           /private/tmp/fakegcp.log /private/tmp/fakeaws.log /private/tmp/mockway.log; do
  [ -f "$log" ] || continue
  if grep -iE 'panic:|runtime error:|recovered from panic|nil pointer dereference' "$log" >> "$PANIC_LOG" 2>/dev/null; then
    echo "PANIC in $log:"
    grep -iE 'panic:|runtime error:|recovered from panic|nil pointer dereference' "$log" | head -5 | sed 's/^/  /'
  fi
done

panic_lines=$(wc -l < "$PANIC_LOG" | tr -d ' ')
echo "PANIC_LINES=$panic_lines (summary at $PANIC_LOG)"

# S94 selective pitfall restoration. The blanket `git checkout pitfalls/`
# discarded everything — including N13's `learned_from_diff_avoid` entries,
# which are grounded in confirmed deletion-as-fix runs (iter N failed,
# iter N+1 succeeded after removing a resource). Replace with a selective
# merge that keeps only `learned_from_diff_avoid` from the post-sweep file
# and discards `learned` + `learned_from_diff` as sweep noise.
echo
echo "=== N13 durability ==="
if [ ! -x ./bin/pitfall-merge ]; then
  echo "WARN: bin/pitfall-merge not found; falling back to blanket discard"
  git checkout pitfalls/ 2>/dev/null || true
  n13_total=0
else
  n13_total=0
  for c in aws gcp scaleway; do
    out=$(./bin/pitfall-merge \
      --pre "$PRE/$c.yaml" \
      --post "pitfalls/$c.yaml" \
      --out "pitfalls/$c.yaml" \
      --keep learned_from_diff_avoid 2>&1)
    echo "  $c: $out"
    added=$(echo "$out" | grep -oE 'kept_new=[0-9]+' | sed 's/kept_new=//')
    n13_total=$((n13_total + ${added:-0}))
  done
fi

echo "N13_EMISSIONS=$n13_total"
if [ "$n13_total" = "0" ]; then
  echo "WARN: zero learned_from_diff_avoid emissions this sweep — N13 silent. Could be (a) the LLM stopped making deletion-recoverable mistakes, or (b) N13 broken. Cross-reference next sweep before treating as a regression."
fi

# Exit non-zero if any panic surfaced — that's a real regression.
if [ "$panic_lines" -gt 0 ]; then
  echo "FAIL: panic-gate detected $panic_lines line(s); see $PANIC_LOG"
  exit 2
fi
