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

while IFS= read -r sc; do
  name="${sc%.yaml}"
  log="$SWEEP_DIR/$name.log"
  ./bin/infrafactory mock reset --config infrafactory.yaml >/dev/null 2>&1 || true
  start=$(date +%s)
  ./bin/infrafactory run "scenarios/training/$sc" --config infrafactory.yaml > "$log" 2>&1
  ec=$?
  dur=$(( $(date +%s) - start ))
  rj=$(ls -td .infrafactory/runs/$name/*/ 2>/dev/null | head -1)run.json
  terminal=$(grep -oE '"terminal_reason":[ ]*"[a-z_]+"' "$rj" 2>/dev/null | head -1 | sed 's/.*"\([a-z_]*\)"/\1/')
  status=$(grep -oE '"status":[ ]*"[a-z_]+"' "$rj" 2>/dev/null | head -1 | sed 's/.*"\([a-z_]*\)"/\1/')
  iters=$(grep -c '"event":"iteration_start"' "$log" 2>/dev/null || echo 0)
  echo -e "$name\t${terminal:-unknown}\t${status:-unknown}\t$iters\t$dur" >> "$SUMMARY"
  printf "%-32s %-22s %-9s iters=%-2s dur=%ss ec=%s\n" "$name" "${terminal:-unknown}" "${status:-unknown}" "$iters" "$dur" "$ec"
done < "$SWEEP_DIR/scenarios.txt"

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
echo "PASS=$pass / TOTAL=$total"

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
