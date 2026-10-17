#!/usr/bin/env bash
# Tests for scenario_change_gate.sh and the scenario-gate job's timeout.
#
# A hung scenario must be labelled `timeout`, not a generic failure, and the
# job's timeout must outlast the scenario budget it is supposed to contain.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/scenario_change_gate.sh"
WORKFLOW="${REPO_ROOT}/.github/workflows/scenario-gate.yml"
FAILURES=0

# new_repo: a throwaway repo on main; a feature branch adds one scenario and
# a stub `go` that builds a fake infrafactory which sleeps. Prints the path.
new_repo() {
  local dir stub
  dir="$(mktemp -d)"
  stub="${dir}-stub"
  mkdir -p "${stub}"
  printf '#!/bin/bash\nmkdir -p bin\nprintf "#!/bin/bash\\nexec sleep 30\\n" > bin/infrafactory\nchmod +x bin/infrafactory\n' > "${stub}/go"
  chmod +x "${stub}/go"
  git -C "${dir}" init -q -b main
  git -C "${dir}" config user.email t@t.t
  git -C "${dir}" config user.name t
  mkdir -p "${dir}/scenarios/training"
  git -C "${dir}" commit -q --allow-empty -m baseline
  git -C "${dir}" checkout -q -b feature
  echo "name: hang" > "${dir}/scenarios/training/hang.yaml"
  git -C "${dir}" add -A
  git -C "${dir}" commit -qm add-scenario
  echo "${dir}"
}

# check <name> <want> <got>: prints ok or FAIL with the line the run printed.
check() {
  if [[ "$2" == "$3" ]]; then
    echo "  ok    $1"
  else
    echo "  FAIL  $1: want '$2', got '$3'"
    FAILURES=$((FAILURES + 1))
  fi
}

test_timed_out_scenario_reads_timeout() {
  local dir stub results logs status
  dir="$(new_repo)"
  stub="${dir}-stub"
  results="${dir}/results.tsv"
  logs="${dir}/logs"
  (cd "${dir}" && PATH="${stub}:${PATH}" BASE_REF=main PER_SCENARIO_TIMEOUT=1 \
    RESULTS="${results}" LOG_DIR="${logs}" bash "${SCRIPT}" > /dev/null 2>&1)
  status="$(awk -F'\t' '$1 == "hang" { print $2 }' "${results}")"
  check "timed-out scenario row reads timeout" "timeout" "${status}"
  rm -rf "${dir}" "${stub}"
}

# test_job_timeout_outlasts_scenario_budget: the job's timeout-minutes must
# exceed MAX_SCENARIOS x PER_SCENARIO_TIMEOUT / 60, read from the script defaults.
test_job_timeout_outlasts_scenario_budget() {
  local max per budget job_timeout
  max="$(sed -n 's/^MAX_SCENARIOS="${MAX_SCENARIOS:-\([0-9]*\)}"/\1/p' "${SCRIPT}")"
  per="$(sed -n 's/^PER_SCENARIO_TIMEOUT="${PER_SCENARIO_TIMEOUT:-\([0-9]*\)}"/\1/p' "${SCRIPT}")"
  budget=$((max * per / 60))
  job_timeout="$(awk '/^  scenario-gate:/ { f = 1 } f && /timeout-minutes:/ { print $2; exit }' "${WORKFLOW}")"
  if (( job_timeout > budget )); then
    echo "  ok    job timeout ${job_timeout}m exceeds scenario budget ${budget}m"
  else
    echo "  FAIL  job timeout ${job_timeout}m does not exceed scenario budget ${budget}m"
    FAILURES=$((FAILURES + 1))
  fi
}

test_timed_out_scenario_reads_timeout
test_job_timeout_outlasts_scenario_budget

if (( FAILURES > 0 )); then
  echo "${FAILURES} failure(s)"
  exit 1
fi
echo "all passed"
