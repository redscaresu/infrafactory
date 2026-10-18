#!/usr/bin/env bash
# Tests for sweep_39.sh: a run that cannot finish is recorded as timed_out, a
# run that writes nothing is no_result (never an older run.json), a panic in a
# mock log fails the sweep, and Ctrl-C stops the sweep rather than the next
# scenario.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/sweep_39.sh"
FAILURES=0
HUNG_RUN_DEADLINE=3
# The hung run sleeps 600s; a sweep that waited it out would take far longer.
SWEEP_BOUND=60
# A stale run.json this old is never newer than the run's start marker.
STALE_MTIME=200001010000

# The caller's sweep settings must not leak into the sweep under test.
export SCENARIOS_FILE=/does/not/exist

# make_root: a throwaway project. Its fake infrafactory never finishes a
# hang-* scenario, dies before writing anything for broken-*, and passes every
# other one. broken-* also has an old run.json from "an earlier run". Prints
# the path.
make_root() {
  local root
  root="$(mktemp -d)"
  mkdir -p "${root}/bin" "${root}/pitfalls" "${root}/scenarios/training" "${root}/mocklogs"
  for c in aws gcp scaleway; do : > "${root}/pitfalls/${c}.yaml"; done
  printf '#!/usr/bin/env bash\necho kept_avoid=0\n' > "${root}/bin/pitfall-merge"
  cat > "${root}/bin/infrafactory" <<'EOF'
#!/usr/bin/env bash
# Fake infrafactory: `mock reset` succeeds; hang-* marks itself started and never
# finishes; broken-* exits without writing a run.json; anything else passes.
if [ "$1" = mock ]; then exit 0; fi
name="$(basename "$2" .yaml)"
case "$name" in
  hang-*) touch "started-${name}"; exec sleep 600 ;;
  broken-*) exit 1 ;;
esac
mkdir -p ".infrafactory/runs/${name}/1"
echo '{"status":"completed","terminal_reason":"target_reached"}' > ".infrafactory/runs/${name}/1/run.json"
EOF
  chmod +x "${root}/bin/infrafactory" "${root}/bin/pitfall-merge"
  for sc in broken hang quick; do touch "${root}/scenarios/training/${sc}-scenario.yaml"; done
  mkdir -p "${root}/.infrafactory/runs/broken-scenario/0"
  echo '{"status":"completed","terminal_reason":"target_reached"}' > "${root}/.infrafactory/runs/broken-scenario/0/run.json"
  touch -t "${STALE_MTIME}" "${root}/.infrafactory/runs/broken-scenario/0/run.json"
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

root="$(make_root)"
summary="${root}/sweep/summary.tsv"
# row <scenario> <column>: one field of that scenario's summary row.
row() { awk -F'\t' -v n="$1" -v c="$2" '$1 == n {print $c}' "${summary}"; }

start=$SECONDS
run_sweep "${root}"
rc=$?
elapsed=$((SECONDS - start))
expect "hung run is recorded as timed_out" timed_out "$(row hang-scenario 2)"
expect "next scenario runs after the timeout" target_reached "$(row quick-scenario 2)"
expect "a run with no run.json of its own is no_result, not an older run" no_result "$(row broken-scenario 2)"
expect "sweep is not held past the bound" yes "$([ "${elapsed}" -lt "${SWEEP_BOUND}" ] && echo yes || echo no)"
expect "summary has a header and one line per scenario" 4 "$(wc -l < "${summary}" | tr -d ' ')"
expect "sweep exits 0 when no mock log has a panic" 0 "${rc}"

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
rm -rf "${root}"

# The sweep's default grace must exceed infrafactory's cancelKillFallback.
# shellcheck disable=SC2016 # the sed pattern matches the literal ${...} in the script
grace="$(sed -n 's/^SWEEP_KILL_GRACE="\${SWEEP_KILL_GRACE:-\([0-9]*\)}"$/\1/p' "${SCRIPT}")"
fallback="$(sed -n 's/^var cancelKillFallback = \([0-9]*\) \* time.Second$/\1/p' "${REPO_ROOT}/internal/cli/exec_runner.go")"
expect "the sweep's default grace exceeds cancelKillFallback" yes "$([ "${grace:-0}" -gt "${fallback:-0}" ] && echo yes || echo no)"

[[ "${FAILURES}" -eq 0 ]]
