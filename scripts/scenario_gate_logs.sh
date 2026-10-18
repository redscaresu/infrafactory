#!/usr/bin/env bash
# Lists the scenario-gate files that scenario-gate.yml uploads, one path per
# line: copies of the results TSV and of each scenario's log, by name, never a
# directory. The copies go into a fresh directory first, because infrafactory
# runs in its own session, can outlive a cancelled step, and can keep writing
# to a log after it was read: the copies are what is checked and uploaded.
# Every copy is checked for the value of OPENROUTER_API_KEY, and the script
# exits 1 listing nothing if any contains it, if the key is unset, or if a
# file cannot be copied or read. A match against the secret, never a
# redaction: the key's value is never printed, and a file that holds it is
# never uploaded.
#
# Env vars, all required: OPENROUTER_API_KEY; RESULTS and LOG_DIR, the paths
# scenario-gate.yml gives scenario_change_gate.sh.
set -uo pipefail

: "${RESULTS:?}" "${LOG_DIR:?}"

if [ -z "${OPENROUTER_API_KEY:-}" ]; then
  echo "scenario-gate-logs: OPENROUTER_API_KEY is unset, so the logs cannot be checked for it; listing nothing" >&2
  exit 1
fi

STAGE="$(mktemp -d)" || exit 1
files=()

# stage <file>: copies <file> into the staging dir under its own name and
# records the copy; a copy that fails lists nothing.
stage() {
  cp "$1" "$STAGE/" || { echo "scenario-gate-logs: could not copy $1; listing nothing" >&2; exit 1; }
  files+=("$STAGE/$(basename "$1")")
}

[ -f "$RESULTS" ] && stage "$RESULTS"
for log in "$LOG_DIR"/*.log; do
  [ -f "$log" ] && stage "$log"
done
# bash 3.2 treats an empty array as unbound under set -u.
[ "${#files[@]}" -gt 0 ] || exit 0

rc=0
for f in "${files[@]}"; do
  # The pattern comes from a process substitution, so the key is never on a
  # command line. grep -q: 0 match, 1 no match, anything else an error.
  grep -qF -f <(printf '%s\n' "$OPENROUTER_API_KEY") -- "$f"
  case $? in
    0) echo "scenario-gate-logs: $f contains the OPENROUTER_API_KEY value; listing nothing" >&2; rc=1 ;;
    1) ;;
    *) echo "scenario-gate-logs: could not read $f; listing nothing" >&2; rc=1 ;;
  esac
done
[ "$rc" -eq 0 ] || exit "$rc"
printf '%s\n' "${files[@]}"
