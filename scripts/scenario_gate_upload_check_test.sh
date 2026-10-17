#!/usr/bin/env bash
# Tests for scenario_gate_upload_check.sh. A file that holds the key must stop
# the upload, and the failure output must not repeat the key.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/scenario_gate_upload_check.sh"
FAKE_KEY="sk-or-test-0000000000"
FAILURES=0

# new_dirs: a scratch dir with a results file and one clean scenario log.
# Prints the path; the caller removes it.
new_dirs() {
  local dir
  dir="$(mktemp -d)"
  mkdir -p "${dir}/logs"
  printf 'scenario\tstatus\n' > "${dir}/results.tsv"
  echo "run finished, no secrets here" > "${dir}/logs/clean.log"
  echo "${dir}"
}

# run_check <dir> <key>: runs the real script; leaves its output in $OUT.
run_check() {
  OUT="$(RESULTS="$1/results.tsv" LOG_DIR="$1/logs" OPENROUTER_API_KEY="$2" \
    bash "${SCRIPT}" 2>&1)"
  RC=$?
}

check() {
  if [[ "$2" == "$3" ]]; then
    echo "  ok    $1"
  else
    echo "  FAIL  $1: want '$2', got '$3'"
    FAILURES=$((FAILURES + 1))
  fi
}

test_clean_files_are_listed() {
  local dir
  dir="$(new_dirs)"
  run_check "${dir}" "${FAKE_KEY}"
  check "clean files exit 0" "0" "${RC}"
  check "clean files are listed" "2" "$(printf '%s\n' "${OUT}" | grep -c "${dir}")"
  rm -rf "${dir}"
}

test_seeded_key_fails_and_is_not_printed() {
  local dir
  dir="$(new_dirs)"
  echo "leaked ${FAKE_KEY} in the log" > "${dir}/logs/leak.log"
  run_check "${dir}" "${FAKE_KEY}"
  check "a file holding the key exits non-zero" "1" "$([ "${RC}" -ne 0 ] && echo 1 || echo 0)"
  check "the failure does not repeat the key" "0" "$(printf '%s' "${OUT}" | grep -c "${FAKE_KEY}")"
  rm -rf "${dir}"
}

test_empty_key_fails_closed() {
  local dir
  dir="$(new_dirs)"
  run_check "${dir}" ""
  check "an empty key exits non-zero" "1" "$([ "${RC}" -ne 0 ] && echo 1 || echo 0)"
  check "an empty key says so instead of blaming a file" "1" "$(printf '%s' "${OUT}" | grep -c 'is empty')"
  rm -rf "${dir}"
}

test_clean_files_are_listed
test_seeded_key_fails_and_is_not_printed
test_empty_key_fails_closed

if (( FAILURES > 0 )); then
  echo "${FAILURES} failure(s)"
  exit 1
fi
echo "all passed"
