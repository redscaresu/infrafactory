#!/bin/bash
# M95 — multi-pass auto-learning proof. Runs gcp-full-stack 5 times
# back-to-back, snapshotting pitfalls/gcp.yaml between runs. If
# M86+M90+M91+M92 work as designed, learned-pitfall count should
# grow + iteration count should drop across passes.
set -uo pipefail

SCENARIO="${SCENARIO:-gcp-full-stack}"
PASSES="${PASSES:-5}"
RESULTS=/tmp/m95_results.tsv
LOG_DIR=/tmp/m95_logs
mkdir -p "$LOG_DIR"
: > "$RESULTS"
echo -e "pass\tstatus\titers\tterminal_reason\tseconds\tlearned_before\tlearned_after" >> "$RESULTS"

CLOUD=$(grep -E "^cloud:" scenarios/training/${SCENARIO}.yaml | awk '{print $2}')
PITFALLS_FILE="pitfalls/${CLOUD}.yaml"

# Fresh build to avoid M93-class stale-binary surprises.
echo "scenario-multipass: building bin/infrafactory"
go build -o bin/infrafactory ./cmd/infrafactory
echo

for pass in $(seq 1 "$PASSES"); do
  learned_before=$(grep -c "source: learned" "$PITFALLS_FILE" 2>/dev/null || echo 0)
  echo "[$(date +%H:%M:%S)] pass $pass — learned-pitfalls before: $learned_before"
  start=$(date +%s)
  log="$LOG_DIR/pass${pass}.log"
  perl -e 'alarm shift; exec @ARGV' 1200 ./bin/infrafactory run --clean "scenarios/training/${SCENARIO}.yaml" > "$log" 2>&1
  rc=$?
  end=$(date +%s)
  elapsed=$((end - start))
  learned_after=$(grep -c "source: learned" "$PITFALLS_FILE" 2>/dev/null || echo 0)
  status=$(grep -E "^Status:" "$log" | tail -1 | awk -F': ' '{print $2}' || echo "no-status")
  terminal=$(grep -E "^- run/terminal_reason:" "$log" | tail -1 | sed -E 's/.*pass \((.*)\)/\1/' || echo "no-terminal")
  iters=$(grep -cE "^- run/iteration_[0-9]+_generate:" "$log" || echo "0")
  if [ "$rc" -eq 124 ]; then status="timeout"; terminal="timeout_1200s"; fi
  delta=$((learned_after - learned_before))
  echo "[$(date +%H:%M:%S)] pass $pass: $status ($iters iters, ${elapsed}s, $terminal) — learned: ${learned_before}→${learned_after} (Δ${delta})"
  echo -e "${pass}\t${status}\t${iters}\t${terminal}\t${elapsed}\t${learned_before}\t${learned_after}" >> "$RESULTS"
done

echo
echo "=== M95 multipass — ${SCENARIO} over ${PASSES} passes ==="
column -t -s $'\t' "$RESULTS"
echo
echo "Final learned pitfalls in ${PITFALLS_FILE}:"
grep -c "source: learned" "$PITFALLS_FILE"
