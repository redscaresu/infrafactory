#!/usr/bin/env bash
# Prints the scenario-gate files that may be uploaded, one per line. Fails if
# OPENROUTER_API_KEY is unset or any listed file holds its value.
#
# This is a match against the secret, not a redaction: a file that holds the
# key is never uploaded, and the failure names the file, never the key.
set -o pipefail

# shellcheck source=scripts/scenario_gate_paths.sh
. "$(dirname "$0")/scenario_gate_paths.sh"

if [ -z "${OPENROUTER_API_KEY:-}" ]; then
  echo "scenario-gate: OPENROUTER_API_KEY is empty; cannot check the upload" >&2
  exit 1
fi

files=()
[ -f "$RESULTS" ] && files+=("$RESULTS")
for log in "$LOG_DIR"/*.log; do
  [ -f "$log" ] && files+=("$log")
done

for f in ${files[@]+"${files[@]}"}; do
  grep -qF -- "$OPENROUTER_API_KEY" "$f"
  case $? in
    0) echo "scenario-gate: $f holds the OPENROUTER_API_KEY value; not uploading" >&2; exit 1 ;;
    1) ;;
    *) echo "scenario-gate: could not scan $f" >&2; exit 1 ;;
  esac
done

for f in ${files[@]+"${files[@]}"}; do
  echo "$f"
done
