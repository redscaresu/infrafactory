#!/bin/bash
# M94 — re-run the 11 AWS scenarios with SeaweedFS up + the full
# M86+M90+M91+M92 chain live. This is the real auto-learning proof
# for AWS — M93 failed because SeaweedFS was down (s3 reset error
# before LLM was even invoked).
set -uo pipefail

RESULTS=/tmp/m94_results.tsv
LOG_DIR=/tmp/m94_logs
mkdir -p "$LOG_DIR"
: > "$RESULTS"
echo -e "scenario\tstatus\titers\tterminal_reason\tseconds" >> "$RESULTS"

AWS=(
  aws-dynamodb aws-eks aws-full-stack aws-iam aws-instance aws-rds
  aws-route53 aws-s3 aws-secrets-manager aws-sqs aws-vpc-network
)

total_start=$(date +%s)
for name in "${AWS[@]}"; do
  f="scenarios/training/${name}.yaml"
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
  echo "[$(date +%H:%M:%S)] $name: $status ($iters iters, ${elapsed}s, $terminal) — aws-learned: $(grep -c 'source: learned' pitfalls/aws.yaml)" >&2
done
total_end=$(date +%s)
echo
echo "=== M94 AWS proof results ==="
column -t -s $'\t' "$RESULTS"
echo
awk -F'\t' 'NR>1 {print $2}' "$RESULTS" | sort | uniq -c
echo "aws learned: $(grep -c 'source: learned' pitfalls/aws.yaml)"
