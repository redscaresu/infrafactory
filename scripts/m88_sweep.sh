#!/bin/bash
# M88 — sweep every scenario in scenarios/training/.
# Writes TSV to /tmp/m88_results.tsv: scenario \t status \t iters \t terminal_reason \t seconds
set -uo pipefail

RESULTS=/tmp/m88_results.tsv
LOG_DIR=/tmp/m88_logs
mkdir -p "$LOG_DIR"
: > "$RESULTS"
echo -e "scenario\tstatus\titers\tterminal_reason\tseconds" >> "$RESULTS"

total_start=$(date +%s)
for f in scenarios/training/*.yaml; do
  name=$(basename "$f" .yaml)
  echo "[$(date +%H:%M:%S)] starting $name" >&2
  start=$(date +%s)
  log="$LOG_DIR/$name.log"
  # Per-scenario 12-min cap (full-stacks take ~5 min worst case at 5 iters).
  # macOS has no `timeout` binary; use a portable perl alarm wrapper.
  perl -e 'alarm shift; exec @ARGV' 720 ./bin/infrafactory run --clean "$f" > "$log" 2>&1
  rc=$?
  end=$(date +%s)
  elapsed=$((end - start))

  # Parse last summary block from log.
  status=$(grep -E "^Status:" "$log" | tail -1 | awk -F': ' '{print $2}' || echo "no-status")
  terminal=$(grep -E "^- run/terminal_reason:" "$log" | tail -1 | sed -E 's/.*pass \((.*)\)/\1/' || echo "no-terminal")
  iters=$(grep -cE "^- run/iteration_[0-9]+_generate:" "$log" || echo "0")

  if [ "$rc" -eq 124 ]; then
    status="timeout"
    terminal="timeout_720s"
  fi

  echo -e "${name}\t${status}\t${iters}\t${terminal}\t${elapsed}" >> "$RESULTS"
  echo "[$(date +%H:%M:%S)] $name: $status ($iters iters, ${elapsed}s, $terminal)" >&2
done
total_end=$(date +%s)
echo "[$(date +%H:%M:%S)] sweep complete: $((total_end - total_start))s total" >&2
echo
echo "=== summary ==="
column -t -s $'\t' "$RESULTS"
echo
echo "=== pass/fail counts ==="
awk -F'\t' 'NR>1 {print $2}' "$RESULTS" | sort | uniq -c | sort -rn
