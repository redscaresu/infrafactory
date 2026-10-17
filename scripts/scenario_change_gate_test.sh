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

# new_repo: a throwaway repo on main; a feature branch adds one scenario.
# Prints the path. The stub `go` is a no-op, so the test's own fake binary stays.
new_repo() {
  local dir stub
  dir="$(mktemp -d)"
  stub="${dir}-stub"
  mkdir -p "${stub}" "${dir}/bin"
  printf '#!/bin/bash\nexit 0\n' > "${stub}/go"
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

# write_fake_infrafactory <path>: a hung run. Its child runs in its own process
# group (set -m, as the real run's Setpgid does) and is stopped only by the
# SIGTERM handler, so a wrapper that sends KILL leaves the child alive.
write_fake_infrafactory() {
  cat > "$1" <<'EOF'
#!/bin/bash
set -m
sleep 30 & child=$!
echo "${child}" > "${CHILD_PID_FILE}"
trap 'kill "${child}" 2>/dev/null; exit 1' TERM
wait
EOF
  chmod +x "$1"
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
  local dir results pid alive rows status iters
  dir="$(new_repo)"
  results="${dir}/results.tsv"
  write_fake_infrafactory "${dir}/bin/infrafactory"
  (cd "${dir}" && PATH="${dir}-stub:${PATH}" BASE_REF=main PER_SCENARIO_TIMEOUT=1 KILL_AFTER=5 \
    CHILD_PID_FILE="${dir}/child.pid" RESULTS="${results}" LOG_DIR="${dir}/logs" \
    bash "${SCRIPT}" > /dev/null 2>&1)
  rows="$(wc -l < "${results}" | tr -d ' ')"
  status="$(awk -F'\t' '$1 == "hang" { print $2 }' "${results}")"
  iters="$(awk -F'\t' '$1 == "hang" { print $3 }' "${results}")"
  pid="$(cat "${dir}/child.pid")"
  sleep 1
  if kill -0 "${pid}" 2>/dev/null; then alive=yes; else alive=no; fi
  check "one header and one row per scenario, no phantom row" "2" "${rows}"
  check "timed-out scenario row reads timeout" "timeout" "${status}"
  check "timed-out scenario reports zero iterations" "0" "${iters}"
  check "timed-out scenario's child is stopped" "no" "${alive}"
  rm -rf "${dir}" "${dir}-stub"
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
