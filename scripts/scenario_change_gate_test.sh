#!/usr/bin/env bash
# Tests for scenario_change_gate.sh and scenario_gate_leak_check.sh.
#
# A hung scenario must read "timeout" in the results, and a file holding the
# API key must fail the upload check. Both run the real scripts; only `go`
# is stubbed, so the gate runs a stand-in scenario that sleeps.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="${REPO_ROOT}/scripts/scenario_change_gate.sh"
LEAK_CHECK="${REPO_ROOT}/scripts/scenario_gate_leak_check.sh"
FAKE_KEY="sk-fake-test-key-0001"
FAILURES=0

# check <name> <want> <got>
check() {
  if [[ "$2" == "$3" ]]; then
    echo "  ok    $1"
  else
    echo "  FAIL  $1: want '$2', got '$3'"
    FAILURES=$((FAILURES + 1))
  fi
}

# new_stub_go <dir>: a `go` that stands in for `go build`, writing a
# scenario binary that sleeps far past the test's 1-second timeout.
new_stub_go() {
  local dir="$1"
  mkdir -p "${dir}"
  cat > "${dir}/go" <<'EOF'
#!/bin/bash
mkdir -p bin
printf '#!/bin/bash\nexec sleep 30\n' > bin/infrafactory
chmod +x bin/infrafactory
EOF
  chmod +x "${dir}/go"
}

test_timeout_row() {
  local repo out stub base
  repo="$(mktemp -d)"
  out="$(mktemp -d)"
  stub="$(mktemp -d)"
  git -C "${repo}" init -q
  git -C "${repo}" config user.email t@t.t
  git -C "${repo}" config user.name t
  echo base > "${repo}/README.md"
  git -C "${repo}" add -A
  git -C "${repo}" commit -qm base
  base="$(git -C "${repo}" rev-parse HEAD)"
  mkdir -p "${repo}/scenarios/training"
  echo "name: slow" > "${repo}/scenarios/training/slow.yaml"
  git -C "${repo}" add -A
  git -C "${repo}" commit -qm "add slow scenario"
  new_stub_go "${stub}"

  (cd "${repo}" && PATH="${stub}:${PATH}" BASE_REF="${base}" PER_SCENARIO_TIMEOUT=1 \
    SCENARIO_GATE_OUT_DIR="${out}" bash "${GATE}" > "${out}/gate.out" 2>&1)

  local row
  row="$(awk -F'\t' '$1 == "slow" { print $2 }' "${out}/scenario_gate_results.tsv")"
  check "timed-out scenario reads timeout" "timeout" "${row}"
  rm -rf "${repo}" "${out}" "${stub}"
}

test_leak_check() {
  local out rc output
  out="$(mktemp -d)"
  mkdir -p "${out}/scenario_gate_logs"
  printf 'Status: success\n' > "${out}/scenario_gate_results.tsv"
  printf 'iteration 1 ok\n' > "${out}/scenario_gate_logs/clean.log"

  rc=0
  SCENARIO_GATE_OUT_DIR="${out}" OPENROUTER_API_KEY="${FAKE_KEY}" \
    bash "${LEAK_CHECK}" > /dev/null 2>&1 || rc=$?
  check "clean files pass the leak check" "0" "${rc}"

  printf 'env dump: %s\n' "${FAKE_KEY}" > "${out}/scenario_gate_logs/leaky.log"
  rc=0
  output="$(SCENARIO_GATE_OUT_DIR="${out}" OPENROUTER_API_KEY="${FAKE_KEY}" \
    bash "${LEAK_CHECK}" 2>&1)" || rc=$?
  check "a log seeded with the key fails the leak check" "1" "${rc}"
  check "the leak check does not print the key" "0" "$(grep -c -F -- "${FAKE_KEY}" <<< "${output}")"

  rc=0
  SCENARIO_GATE_OUT_DIR="${out}" OPENROUTER_API_KEY="" \
    bash "${LEAK_CHECK}" > /dev/null 2>&1 || rc=$?
  check "an empty key fails closed" "1" "${rc}"
  rm -rf "${out}"
}

test_timeout_row
test_leak_check

if (( FAILURES > 0 )); then
  echo "${FAILURES} case(s) failed"
  exit 1
fi
echo "all cases passed"
