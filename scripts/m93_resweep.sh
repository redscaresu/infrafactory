#!/bin/bash
# M93 — re-sweep the 19 failed scenarios from M88 with the
# M86+M90+M91+M92 chain live. AWS is the main proof point (M92).
set -uo pipefail

RESULTS=/tmp/m93_results.tsv
LOG_DIR=/tmp/m93_logs
mkdir -p "$LOG_DIR"
: > "$RESULTS"
echo -e "scenario\tstatus\titers\tterminal_reason\tseconds" >> "$RESULTS"

FAILED=(
  aws-dynamodb aws-eks aws-full-stack aws-iam aws-instance aws-rds
  aws-route53 aws-s3 aws-secrets-manager aws-sqs aws-vpc-network
  gcp-cloud-sql gcp-full-stack gcp-gke-cluster gcp-iam
  gcp-load-balancer gcp-storage gcp-vm-network
  private-lb-db-paris
)

total_start=$(date +%s)
for name in "${FAILED[@]}"; do
  f="scenarios/training/${name}.yaml"
  if [ ! -f "$f" ]; then echo "SKIP missing: $f" >&2; continue; fi
  echo "[$(date +%H:%M:%S)] starting $name" >&2
  start=$(date +%s)
  log="$LOG_DIR/$name.log"
  perl -e 'alarm shift; exec @ARGV' 720 ./bin/infrafactory run --clean "$f" > "$log" 2>&1
  rc=$?
  end=$(date +%s)
  elapsed=$((end - start))
  status=$(grep -E "^Status:" "$log" | tail -1 | awk -F': ' '{print $2}' || echo "no-status")
  terminal=$(grep -E "^- run/terminal_reason:" "$log" | tail -1 | sed -E 's/.*pass \((.*)\)/\1/' || echo "no-terminal")
  iters=$(grep -cE "^- run/iteration_[0-9]+_generate:" "$log" || echo "0")
  if [ "$rc" -eq 124 ]; then status="timeout"; terminal="timeout_720s"; fi
  echo -e "${name}\t${status}\t${iters}\t${terminal}\t${elapsed}" >> "$RESULTS"
  echo "[$(date +%H:%M:%S)] $name: $status ($iters iters, ${elapsed}s, $terminal)" >&2
done
total_end=$(date +%s)
echo "[$(date +%H:%M:%S)] re-sweep complete: $((total_end - total_start))s total" >&2
echo
echo "=== M93 re-sweep results ==="
column -t -s $'\t' "$RESULTS"
echo
echo "=== pass/fail vs M88 ==="
awk -F'\t' 'NR>1 {print $2}' "$RESULTS" | sort | uniq -c
echo
echo "=== learned pitfalls after re-sweep ==="
for c in aws gcp scaleway; do
  echo -n "  pitfalls/$c.yaml: "
  grep -c "source: learned" pitfalls/$c.yaml
done
