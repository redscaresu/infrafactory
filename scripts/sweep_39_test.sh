#!/usr/bin/env bash
# Tests for sweep_39.sh's per-scenario deadline. A run that cannot finish is
# recorded as timed_out, and the sweep goes on to the next scenario.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/sweep_39.sh"
FAILURES=0
HUNG_RUN_DEADLINE=3
# The hung run sleeps 600s; a sweep that waited it out would take far longer.
SWEEP_BOUND=60

# make_root: a throwaway project whose fake infrafactory never finishes a
# hang-* scenario and passes every other one. Prints its path.
make_root() {
  local root
  root="$(mktemp -d)"
  mkdir -p "${root}/bin" "${root}/pitfalls" "${root}/scenarios/training"
  for c in aws gcp scaleway; do : > "${root}/pitfalls/${c}.yaml"; done
  printf '#!/usr/bin/env bash\necho kept_avoid=0\n' > "${root}/bin/pitfall-merge"
  cat > "${root}/bin/infrafactory" <<'EOF'
#!/usr/bin/env bash
# Fake infrafactory: `mock reset` succeeds; a hang-* scenario never finishes.
if [ "$1" = mock ]; then exit 0; fi
name="$(basename "$2" .yaml)"
case "$name" in
  hang-*) exec sleep 600 ;;
esac
mkdir -p ".infrafactory/runs/${name}/1"
echo '{"status":"completed","terminal_reason":"target_reached"}' > ".infrafactory/runs/${name}/1/run.json"
EOF
  chmod +x "${root}/bin/infrafactory" "${root}/bin/pitfall-merge"
  touch "${root}/scenarios/training/hang-scenario.yaml" "${root}/scenarios/training/quick-scenario.yaml"
  echo "${root}"
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
root="$(make_root)"
start=$SECONDS
ROOT="${root}" SWEEP_DIR="${root}/sweep" SWEEP_RUN_TIMEOUT="${HUNG_RUN_DEADLINE}" \
  bash "${SCRIPT}" > "${root}/out.txt" 2>&1
elapsed=$((SECONDS - start))
summary="${root}/sweep/summary.tsv"
# row <scenario> <column>: one field of that scenario's summary row.
row() { awk -F'\t' -v n="$1" -v c="$2" '$1 == n {print $c}' "${summary}"; }

expect "hung run is recorded as timed_out" timed_out "$(row hang-scenario 2)"
expect "next scenario runs after the timeout" target_reached "$(row quick-scenario 2)"
expect "sweep is not held past the bound" yes "$([ "${elapsed}" -lt "${SWEEP_BOUND}" ] && echo yes || echo no)"
expect "summary has a header and one line per scenario" 3 "$(wc -l < "${summary}" | tr -d ' ')"
rm -rf "${root}"

[[ "${FAILURES}" -eq 0 ]]
