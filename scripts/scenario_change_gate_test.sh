#!/usr/bin/env bash
# Tests for scenario_change_gate.sh and the scenario-gate job's timeout.
#
# A timed-out scenario must read `timeout` whichever way its run ended, and no
# child of the run may outlive the escalation. The job timeout must outlast the
# worst case the script allows per scenario, read from the script's own defaults.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/scenario_change_gate.sh"
WORKFLOW="${REPO_ROOT}/.github/workflows/scenario-gate.yml"
FAILURES=0
GATE_WAIT_SECS=20

# Child shapes for the fake run. A child that ignores TERM survives the TERM and
# is stopped only by the escalation; a plain child takes TERM.
TERM_IGNORING_CHILD='(trap "" TERM; exec sleep 30) & child=$!'
PLAIN_CHILD='sleep 30 & child=$!'

if ! command -v pgrep > /dev/null; then
  echo "scenario_change_gate_test: pgrep is required (procps on Linux)" >&2
  exit 2
fi

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

# write_fake_infrafactory <path> <parent-trap-action> <child-command>: a hung
# run. Its SIGTERM handler is <parent-trap-action> (empty ignores TERM), and it
# starts <child-command>, which sets $child and must not block.
write_fake_infrafactory() {
  {
    echo '#!/bin/bash'
    echo "trap '$2' TERM"
    echo "$3"
    echo 'echo "${child}" > "${CHILD_PID_FILE}"'
    echo 'wait'
  } > "$1"
  chmod +x "$1"
}

# is_number <value>: true for a non-empty run of digits.
is_number() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

# default_of <VAR>: the numeric default the gate script gives VAR.
default_of() {
  sed -n "s/^$1=\"\${$1:-\([0-9]*\)}\"/\1/p" "${SCRIPT}"
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

# kill_tree <pid>: kills a process and everything below it, for cleanup when
# the gate under test never returns.
kill_tree() {
  local child
  for child in $(pgrep -P "$1"); do
    kill_tree "${child}"
  done
  kill -KILL "$1" 2>/dev/null
}

# run_hung_scenario <parent-action> <child-command>: runs the gate on one hung
# scenario in the background and polls it, so a gate that never returns fails
# the test instead of hanging it. Leaves results in $dir/results.tsv and the
# child's pid in $dir/child.pid. RETURNED is 1 if the gate returned in time.
run_hung_scenario() {
  local gate i
  dir="$(new_repo)"
  write_fake_infrafactory "${dir}/bin/infrafactory" "$1" "$2"
  (cd "${dir}" && exec env PATH="${dir}-stub:${PATH}" BASE_REF=main PER_SCENARIO_TIMEOUT=3 KILL_AFTER=1 \
    CHILD_PID_FILE="${dir}/child.pid" RESULTS="${dir}/results.tsv" LOG_DIR="${dir}/logs" \
    bash "${SCRIPT}" > /dev/null 2>&1) &
  gate=$!
  RETURNED=0
  for ((i = 0; i < GATE_WAIT_SECS; i++)); do
    if ! kill -0 "${gate}" 2>/dev/null; then
      RETURNED=1
      break
    fi
    sleep 1
  done
  if (( RETURNED == 0 )); then kill_tree "${gate}"; fi
}

# report_hung_scenario <label>: checks the gate returned, the results, and that
# the child is gone.
report_hung_scenario() {
  local pid alive rows status iters
  check "$1: the gate returned within ${GATE_WAIT_SECS}s" "1" "${RETURNED}"
  rows="$(wc -l < "${dir}/results.tsv" 2>/dev/null | tr -d ' ')"
  status="$(awk -F'\t' '$1 == "hang" { print $2 }' "${dir}/results.tsv" 2>/dev/null)"
  iters="$(awk -F'\t' '$1 == "hang" { print $3 }' "${dir}/results.tsv" 2>/dev/null)"
  pid="$(cat "${dir}/child.pid" 2>/dev/null)"
  sleep 1
  if is_number "${pid}" && ! kill -0 "${pid}" 2>/dev/null; then alive=no; else alive=yes; fi
  check "$1: one header and one row per scenario, no phantom row" "2" "${rows}"
  check "$1: timed-out scenario row reads timeout" "timeout" "${status}"
  check "$1: timed-out scenario reports zero iterations" "0" "${iters}"
  check "$1: the child was recorded" "yes" "$(is_number "${pid}" && echo yes || echo no)"
  check "$1: the child is stopped" "no" "${alive}"
  rm -rf "${dir}" "${dir}-stub"
}

# The realistic case: the run exits on TERM, as infrafactory does after its
# guard, and its child ignores TERM. The gate must still stop the child.
test_parent_exits_child_ignores_term() {
  run_hung_scenario 'exit 1' "${TERM_IGNORING_CHILD}"
  report_hung_scenario "parent exits on TERM, child ignores TERM"
}

# The run ignores TERM and its child takes it: only the escalation stops the run.
test_parent_ignores_term() {
  run_hung_scenario '' "${PLAIN_CHILD}"
  report_hung_scenario "parent ignores TERM"
}

# The run exits on TERM and its child takes TERM too: the child must be stopped
# by the gate, not by the stub.
test_parent_exits_child_takes_term() {
  run_hung_scenario 'exit 1' "${PLAIN_CHILD}"
  report_hung_scenario "parent exits on TERM, child takes TERM"
}

# test_gate_term_stops_run: a TERM to the gate itself stops the run and its
# child, and the gate exits 143.
test_gate_term_stops_run() {
  local gate i rc alive pid
  dir="$(new_repo)"
  write_fake_infrafactory "${dir}/bin/infrafactory" 'exit 1' "${TERM_IGNORING_CHILD}"
  (cd "${dir}" && exec env PATH="${dir}-stub:${PATH}" BASE_REF=main PER_SCENARIO_TIMEOUT=60 KILL_AFTER=1 \
    CHILD_PID_FILE="${dir}/child.pid" RESULTS="${dir}/results.tsv" LOG_DIR="${dir}/logs" \
    bash "${SCRIPT}" > /dev/null 2>&1) &
  gate=$!
  i=0
  while [[ ! -s "${dir}/child.pid" ]] && (( i < GATE_WAIT_SECS )); do
    sleep 1
    i=$((i + 1))
  done
  kill -TERM "${gate}" 2>/dev/null
  wait "${gate}"
  rc=$?
  pid="$(cat "${dir}/child.pid" 2>/dev/null)"
  sleep 1
  if is_number "${pid}" && ! kill -0 "${pid}" 2>/dev/null; then alive=no; else alive=yes; fi
  check "gate TERM exits 143" "143" "${rc}"
  check "gate TERM: the child is stopped" "no" "${alive}"
  rm -rf "${dir}" "${dir}-stub"
}

# test_job_timeout_outlasts_scenario_budget: the job's timeout-minutes must
# exceed MAX_SCENARIOS x (PER_SCENARIO_TIMEOUT + KILL_AFTER) / 60, the worst case
# per scenario, with every number read from the script and the workflow.
test_job_timeout_outlasts_scenario_budget() {
  local max per kill_after job_timeout budget
  max="$(default_of MAX_SCENARIOS)"
  per="$(default_of PER_SCENARIO_TIMEOUT)"
  kill_after="$(default_of KILL_AFTER)"
  job_timeout="$(awk '/^  scenario-gate:/ { f = 1 } f && /timeout-minutes:/ { print $2; exit }' "${WORKFLOW}")"
  if ! is_number "${max}" || ! is_number "${per}" || ! is_number "${kill_after}" || ! is_number "${job_timeout}"; then
    echo "  FAIL  could not read MAX_SCENARIOS, PER_SCENARIO_TIMEOUT, KILL_AFTER and the job timeout (got '${max}' '${per}' '${kill_after}' '${job_timeout}')"
    FAILURES=$((FAILURES + 1))
    return
  fi
  budget=$((max * (per + kill_after) / 60))
  if (( job_timeout > budget )); then
    echo "  ok    job timeout ${job_timeout}m exceeds scenario budget ${budget}m"
  else
    echo "  FAIL  job timeout ${job_timeout}m does not exceed scenario budget ${budget}m"
    FAILURES=$((FAILURES + 1))
  fi
}

test_parent_exits_child_ignores_term
test_parent_ignores_term
test_parent_exits_child_takes_term
test_gate_term_stops_run
test_job_timeout_outlasts_scenario_budget

if (( FAILURES > 0 )); then
  echo "${FAILURES} failure(s)"
  exit 1
fi
echo "all passed"
