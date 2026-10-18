#!/usr/bin/env bash
# Tests for sweep_39.sh: a run that cannot finish is recorded as timed_out, a
# run that dies after its startup write is crashed, a run that writes nothing
# is no_result (never an older run.json), a panic in a mock log fails the
# sweep, a mock reset that never returns stops the sweep with exit 3, and Ctrl-C
# stops the sweep rather than the next scenario and restores the pitfalls on the
# way out.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/sweep_39.sh"
FAILURES=0
HUNG_RUN_DEADLINE=3
# The hung run sleeps 600s; a sweep that waited it out would take far longer.
SWEEP_BOUND=60
# A stale run.json this old is never newer than the run's start marker.
STALE_MTIME=200001010000
# Scenarios: broken, hang, quick, silent (sorted by ls).
SCENARIO_COUNT=4

# make_root: a throwaway project. Its fake infrafactory never finishes a
# hang-* scenario (it first appends a fix line to pitfalls/aws.yaml), writes a
# running run.json and dies for broken-*, dies with nothing written for
# silent-*, and passes every other one. `mock reset` hangs on the call numbered
# in the reset-hang-at file, if there is one. flaky-* fails in a transport shape
# the sweep retries. Prints the path.
make_root() {
  local root
  root="$(mktemp -d)"
  mkdir -p "${root}/bin" "${root}/pitfalls" "${root}/scenarios/training" "${root}/mocklogs"
  for c in aws gcp scaleway; do : > "${root}/pitfalls/${c}.yaml"; done
  # Fake pitfall-merge: restores the pre-sweep file, discarding this sweep's additions.
  cat > "${root}/bin/pitfall-merge" <<'EOF'
#!/usr/bin/env bash
while [ $# -gt 0 ]; do
  case "$1" in
    --pre) pre="$2"; shift ;;
    --out) out="$2"; shift ;;
  esac
  shift
done
cp "$pre" "$out"
echo kept_avoid=0
EOF
  cat > "${root}/bin/infrafactory" <<'EOF'
#!/usr/bin/env bash
# Fake infrafactory: `mock reset` succeeds; hang-* learns a fix and never
# finishes; broken-* writes its startup run.json (status running) and dies;
# silent-* dies before writing anything; anything else passes.
if [ "$1" = mock ]; then
  n=$(( $(cat reset-count 2>/dev/null || echo 0) + 1 ))
  echo "${n}" > reset-count
  if [ -f reset-hang-at ] && [ "$(cat reset-hang-at)" = "${n}" ]; then
    touch reset-hung
    exec sleep 600
  fi
  exit 0
fi
name="$(basename "$2" .yaml)"
case "$name" in
  hang-*)
    echo "- source: fix" >> pitfalls/aws.yaml
    touch "started-${name}"
    exec sleep 600 ;;
  flaky-*)
    mkdir -p ".infrafactory/runs/${name}/1"
    echo '{"status":"completed","terminal_reason":"repair_budget_exhausted"}' > ".infrafactory/runs/${name}/1/run.json"
    echo '{"event":"iteration_start"}'
    echo '{"event":"stage_end","status":"failed","stage":"iteration_1_generate"}'
    exit 1 ;;
  broken-*)
    mkdir -p ".infrafactory/runs/${name}/1"
    echo '{"status":"running"}' > ".infrafactory/runs/${name}/1/run.json"
    exit 1 ;;
  silent-*) exit 1 ;;
esac
mkdir -p ".infrafactory/runs/${name}/1"
echo '{"status":"completed","terminal_reason":"target_reached"}' > ".infrafactory/runs/${name}/1/run.json"
EOF
  chmod +x "${root}/bin/infrafactory" "${root}/bin/pitfall-merge"
  for sc in broken hang quick silent; do touch "${root}/scenarios/training/${sc}-scenario.yaml"; done
  mkdir -p "${root}/.infrafactory/runs/silent-scenario/0"
  echo '{"status":"completed","terminal_reason":"target_reached"}' > "${root}/.infrafactory/runs/silent-scenario/0/run.json"
  touch -t "${STALE_MTIME}" "${root}/.infrafactory/runs/silent-scenario/0/run.json"
  echo "${root}"
}

# run_sweep <root> [sweep-env...]: runs the sweep with only the given settings,
# so nothing the caller exported reaches it.
run_sweep() {
  local root="$1"
  shift
  env -i PATH="$PATH" ROOT="${root}" SWEEP_DIR="${root}/sweep" \
    MOCK_LOG_ROOT="${root}/mocklogs" SWEEP_RUN_TIMEOUT="${HUNG_RUN_DEADLINE}" \
    "$@" bash "${SCRIPT}" > "${root}/out.txt" 2>&1
}

# start_sweep <root> [sweep-env...]: starts the sweep in the background. env
# execs bash, so $! is the sweep's own pid.
start_sweep() {
  local root="$1"
  shift
  env -i PATH="$PATH" ROOT="${root}" SWEEP_DIR="${root}/sweep" \
    MOCK_LOG_ROOT="${root}/mocklogs" SWEEP_RUN_TIMEOUT="${HUNG_RUN_DEADLINE}" \
    "$@" bash "${SCRIPT}" > "${root}/out.txt" 2>&1 &
}

# wait_bounded <pid>: waits up to SWEEP_BOUND seconds for a background sweep.
# Sets WAITED_STATUS to its exit status, or to still-running after KILLing its
# process group. Set in the caller, not a subshell, so wait can reap the job.
wait_bounded() {
  local pid="$1"
  for _ in $(seq 1 $((SWEEP_BOUND * 10))); do kill -0 "${pid}" 2>/dev/null || break; sleep 0.1; done
  if kill -0 "${pid}" 2>/dev/null; then
    kill -KILL -- "-${pid}" 2>/dev/null
    wait "${pid}" 2>/dev/null
    WAITED_STATUS=still-running
    return
  fi
  wait "${pid}"
  WAITED_STATUS=$?
}

# expect <name> <want> <got>: records a FAIL line when got is not want.
expect() {
  local name="$1" want="$2" got="$3"
  if [[ "${got}" == "${want}" ]]; then
    echo "  ok    ${name}"
  else
    echo "  FAIL  ${name}: expected ${want}, got ${got}"
    FAILURES=$((FAILURES + 1))
  fi
}

echo "sweep_39.sh:"

bad="$(make_root)"
run_sweep "${bad}" SWEEP_RUN_TIMEOUT=30m
expect "a SWEEP_RUN_TIMEOUT that is not whole seconds is refused" 1 "$?"
rm -rf "${bad}"

lone="$(mktemp -d)"
cp "${SCRIPT}" "${lone}/sweep_39.sh"
root="$(make_root)"
env -i PATH="$PATH" ROOT="${root}" SWEEP_DIR="${root}/lone" bash "${lone}/sweep_39.sh" > /dev/null 2>&1
expect "a sweep without its deadline script is refused" 1 "$?"
rm -rf "${lone}"

summary="${root}/sweep/summary.tsv"
# row <scenario> <column>: one field of that scenario's summary row.
row() { awk -F'\t' -v n="$1" -v c="$2" '$1 == n {print $c}' "${summary}"; }

start=$SECONDS
run_sweep "${root}"
rc=$?
elapsed=$((SECONDS - start))
expect "hung run is recorded as timed_out" timed_out "$(row hang-scenario 2)"
expect "next scenario runs after the timeout" target_reached "$(row quick-scenario 2)"
expect "a run that died after its startup write is crashed" crashed "$(row broken-scenario 2)"
expect "a run with no run.json of its own is no_result, not an older run" no_result "$(row silent-scenario 2)"
expect "sweep is not held past the bound" yes "$([ "${elapsed}" -lt "${SWEEP_BOUND}" ] && echo yes || echo no)"
expect "summary has a header and one line per scenario" "$((SCENARIO_COUNT + 1))" "$(wc -l < "${summary}" | tr -d ' ')"
expect "sweep exits 0 when no mock log has a panic" 0 "${rc}"
expect "a finished sweep leaves the fix line out of pitfalls" yes "$([ ! -s "${root}/pitfalls/aws.yaml" ] && echo yes || echo no)"

# A mock reset past its deadline stops the sweep with exit 3, after the blocked
# scenario is recorded as reset_timed_out. The third reset comes after the hang
# scenario has appended its fix line, so the stop must restore the pitfalls.
reset_root="$(make_root)"
echo 3 > "${reset_root}/reset-hang-at"
set -m
start=$SECONDS
start_sweep "${reset_root}" SWEEP_RESET_TIMEOUT=3
reset_pid=$!
wait_bounded "${reset_pid}"
reset_status=${WAITED_STATUS}
set +m
reset_elapsed=$((SECONDS - start))
expect "a mock reset past its deadline stops the sweep with exit 3" 3 "${reset_status}"
expect "the sweep does not wait for a hung mock reset" yes "$([ "${reset_elapsed}" -lt "${SWEEP_BOUND}" ] && echo yes || echo no)"
expect "the stop names the mock reset" yes "$(grep -q 'mock reset did not return' "${reset_root}/out.txt" && echo yes || echo no)"
expect "the blocked scenario is recorded as reset_timed_out" reset_timed_out "$(awk -F'\t' -v n=quick-scenario '$1 == n {print $2}' "${reset_root}/sweep/summary.tsv")"
expect "the stop restores the pitfalls the earlier scenario added" yes "$([ ! -s "${reset_root}/pitfalls/aws.yaml" ] && echo yes || echo no)"
rm -rf "${reset_root}"

# TERM to the sweep during a hung reset ends it at once, through the stop trap,
# instead of after the reset's deadline.
term_root="$(make_root)"
echo 1 > "${term_root}/reset-hang-at"
set -m
start=$SECONDS
start_sweep "${term_root}" SWEEP_RESET_TIMEOUT=600
term_pid=$!
for _ in $(seq 1 100); do [ -f "${term_root}/reset-hung" ] && break; sleep 0.1; done
kill -TERM "${term_pid}"
wait_bounded "${term_pid}"
term_status=${WAITED_STATUS}
set +m
term_elapsed=$((SECONDS - start))
expect "TERM during a hung mock reset ends the sweep by TERM" 143 "${term_status}"
expect "TERM during a hung mock reset does not wait for the reset" yes "$([ "${term_elapsed}" -lt "${SWEEP_BOUND}" ] && echo yes || echo no)"
rm -rf "${term_root}"

# The retry's reset times out after the first attempt ran. The first attempt's
# row is kept, so the scenario that ran is not missing from the summary.
retry_root="$(make_root)"
echo flaky-scenario.yaml > "${retry_root}/one.txt"
echo 2 > "${retry_root}/reset-hang-at"
set -m
start_sweep "${retry_root}" SWEEP_RESET_TIMEOUT=3 SCENARIOS_FILE="${retry_root}/one.txt"
retry_pid=$!
wait_bounded "${retry_pid}"
retry_status=${WAITED_STATUS}
set +m
expect "a timed-out reset on the retry stops the sweep with exit 3" 3 "${retry_status}"
expect "the retried scenario keeps its first attempt's row" repair_budget_exhausted "$(awk -F'\t' -v n=flaky-scenario '$1 == n {print $2}' "${retry_root}/sweep/summary.tsv")"
rm -rf "${retry_root}"

echo "panic: boom" > "${root}/mocklogs/fakegcp.log"
run_sweep "${root}"
expect "a panic in a mock log fails the sweep with exit 2" 2 "$?"
rm -f "${root}/mocklogs/fakegcp.log"

# Ctrl-C is sent to the sweep's whole process group, as a terminal does. Job
# control gives the background sweep its own group and default SIGINT.
rm -f "${root}/started-hang-scenario" "${summary}"
set -m
run_sweep "${root}" SWEEP_RUN_TIMEOUT=600 &
sweep_pid=$!
for _ in $(seq 1 100); do [ -f "${root}/started-hang-scenario" ] && break; sleep 0.1; done
kill -INT -- "-${sweep_pid}"
for _ in $(seq 1 300); do kill -0 "${sweep_pid}" 2>/dev/null || break; sleep 0.1; done
if kill -0 "${sweep_pid}" 2>/dev/null; then
  kill -KILL -- "-${sweep_pid}" 2>/dev/null
  wait "${sweep_pid}" 2>/dev/null
  stopped=still-running
else
  wait "${sweep_pid}"
  stopped=$?
fi
set +m
expect "Ctrl-C stops the sweep with exit 130" 130 "${stopped}"
expect "Ctrl-C does not start the next scenario" no "$(grep -q quick-scenario "${summary}" && echo yes || echo no)"
expect "a stopped sweep leaves the fix line out of pitfalls" yes "$([ ! -s "${root}/pitfalls/aws.yaml" ] && echo yes || echo no)"
rm -rf "${root}"

# The sweep's default grace must exceed infrafactory's cancelKillFallback. A
# value the pattern no longer finds fails the check rather than passing it.
# shellcheck disable=SC2016 # the sed pattern matches the literal ${...} in the script
grace="$(sed -n 's/^SWEEP_KILL_GRACE="\${SWEEP_KILL_GRACE:-\([0-9]*\)}"$/\1/p' "${SCRIPT}")"
fallback="$(sed -n 's/^var cancelKillFallback = \([0-9]*\) \* time.Second$/\1/p' "${REPO_ROOT}/internal/cli/exec_runner.go")"
grace_ok=no
if [[ "${grace}" =~ ^[0-9]+$ && "${fallback}" =~ ^[0-9]+$ && "${grace}" -gt "${fallback}" ]]; then
  grace_ok=yes
fi
expect "the sweep's default grace exceeds cancelKillFallback" yes "${grace_ok}"

[[ "${FAILURES}" -eq 0 ]]
