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

# Per feedback_sweep_protocol.md: discard pitfall additions.
git checkout pitfalls/ 2>/dev/null || true
