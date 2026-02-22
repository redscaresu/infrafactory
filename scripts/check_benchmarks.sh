#!/usr/bin/env bash
set -euo pipefail

if [[ "${INFRAFACTORY_ENABLE_BENCHMARKS:-0}" != "1" ]]; then
  echo "Benchmark checks skipped (set INFRAFACTORY_ENABLE_BENCHMARKS=1 to enable)."
  exit 0
fi

GO_BIN="${GO:-go}"
GOCACHE_DIR="${GOCACHE:-/tmp/infrafactory-gocache}"

MAX_NS_OUTPUT_JSON="${INFRAFACTORY_BENCH_MAX_NS_OUTPUT_JSON:-200000}"
MAX_NS_OUTPUT_HUMAN="${INFRAFACTORY_BENCH_MAX_NS_OUTPUT_HUMAN:-200000}"
MAX_NS_RUNSTORE_RW="${INFRAFACTORY_BENCH_MAX_NS_RUNSTORE_RW:-1000000}"

run_bench_check() {
  local pkg="$1"
  local bench_name="$2"
  local max_ns="$3"

  echo "Running ${bench_name} (${pkg})..."
  local out
  out="$(GOCACHE="${GOCACHE_DIR}" "${GO_BIN}" test "${pkg}" -run '^$' -bench "^${bench_name}$" -benchtime=100x -count=1 -benchmem)"
  echo "${out}"

  local ns
  ns="$(echo "${out}" | awk -v name="${bench_name}" '$1 ~ name"-" {for (i=1; i<=NF; i++) if ($i=="ns/op") {print $(i-1); exit}}')"
  if [[ -z "${ns}" ]]; then
    echo "failed to parse ns/op for ${bench_name}" >&2
    exit 1
  fi

  if ! awk "BEGIN {exit !(${ns} <= ${max_ns})}"; then
    echo "${bench_name} regression: ${ns} ns/op exceeded threshold ${max_ns} ns/op" >&2
    exit 1
  fi
}

run_bench_check "./internal/cli" "BenchmarkOutputContractRenderMachineJSON" "${MAX_NS_OUTPUT_JSON}"
run_bench_check "./internal/cli" "BenchmarkOutputContractRenderHumanSummary" "${MAX_NS_OUTPUT_HUMAN}"
run_bench_check "./internal/runstore" "BenchmarkRunstoreWriteReadMetadata" "${MAX_NS_RUNSTORE_RW}"

echo "Benchmark checks passed."
