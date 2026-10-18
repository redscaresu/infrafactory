#!/usr/bin/env bash
# Fails if any file the scenario-gate artifact upload would send contains the
# OPENROUTER_API_KEY value. It matches the secret and never prints it, and it
# never rewrites a file: a redaction could fail open.
#
# The file list must match the upload `path:` in .github/workflows/scenario-gate.yml.
set -uo pipefail

OUT_DIR="${SCENARIO_GATE_OUT_DIR:-/tmp}"
# Fail closed: an empty key would match every line.
: "${OPENROUTER_API_KEY:?OPENROUTER_API_KEY must be set}"

leaked=0
for f in "$OUT_DIR/scenario_gate_results.tsv" "$OUT_DIR"/scenario_gate_logs/*.log; do
  # A file that does not exist is not uploaded, so it cannot leak.
  [ -f "$f" ] || continue
  grep -F -q -- "$OPENROUTER_API_KEY" "$f"
  case $? in
    0) echo "LEAK: $f contains the OPENROUTER_API_KEY value"; leaked=1 ;;
    1) ;;
    *) echo "leak-check: cannot read $f"; exit 2 ;;
  esac
done

[ "$leaked" -eq 0 ] || exit 1
echo "leak-check: no file to upload contains the key"
