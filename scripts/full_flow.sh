#!/usr/bin/env bash
set -euo pipefail

# One-command full CLI flow:
# 1) mock start
# 2) run scenario (generate/validate/test loop)
# 3) print latest run artifacts and output dir
# 4) mock stop (always, via trap)

SCENARIO_PATH="${1:-scenarios/training/web-app-paris.yaml}"
CONFIG_PATH="${CONFIG_PATH:-infrafactory.yaml}"
OUTPUT_MODE="${OUTPUT_MODE:-human}"
REPAIR_MAX="${REPAIR_MAX:-3}"
CAPTURE_LLM_RAW="${CAPTURE_LLM_RAW:-0}"
RUNSTORE_ROOT="${INFRAFACTORY_RUNSTORE_ROOT:-.infrafactory/runs}"

cleanup() {
  go run ./cmd/infrafactory mock stop --config "${CONFIG_PATH}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[1/4] Starting mock service..."
go run ./cmd/infrafactory mock start --config "${CONFIG_PATH}"

echo "[2/4] Running full flow for scenario: ${SCENARIO_PATH}"
if [[ "${CAPTURE_LLM_RAW}" == "1" ]]; then
  INFRAFACTORY_CAPTURE_LLM_RAW=1 \
    go run ./cmd/infrafactory run "${SCENARIO_PATH}" \
      --config "${CONFIG_PATH}" \
      --repair-iterations-max "${REPAIR_MAX}" \
      --output "${OUTPUT_MODE}"
else
  go run ./cmd/infrafactory run "${SCENARIO_PATH}" \
    --config "${CONFIG_PATH}" \
    --repair-iterations-max "${REPAIR_MAX}" \
    --output "${OUTPUT_MODE}"
fi

echo "[3/4] Resolving latest artifact paths..."
SCENARIO_NAME="$(awk -F': ' '/^scenario:/ {print $2; exit}' "${SCENARIO_PATH}" | tr -d '"')"
if [[ -z "${SCENARIO_NAME}" ]]; then
  echo "Could not derive scenario name from ${SCENARIO_PATH}" >&2
  exit 1
fi

RUN_DIR_BASE="${RUNSTORE_ROOT}/${SCENARIO_NAME}"
LATEST_RUN_DIR="$(ls -1d "${RUN_DIR_BASE}"/* 2>/dev/null | sort | tail -n 1 || true)"
OUTPUT_DIR="output/${SCENARIO_NAME}"

echo "Scenario: ${SCENARIO_NAME}"
echo "Output dir: ${OUTPUT_DIR}"
if [[ -n "${LATEST_RUN_DIR}" ]]; then
  echo "Latest run dir: ${LATEST_RUN_DIR}"
  echo "Run metadata: ${LATEST_RUN_DIR}/run.json"
  echo "Iterations dir: ${LATEST_RUN_DIR}/iterations"
  if compgen -G "${LATEST_RUN_DIR}/iterations/*/llm_raw_*.json" >/dev/null; then
    echo "LLM raw captures:"
    ls -1 "${LATEST_RUN_DIR}"/iterations/*/llm_raw_*.json
  else
    echo "LLM raw captures: none (set CAPTURE_LLM_RAW=1 to enable)"
  fi
else
  echo "No run directory found under ${RUN_DIR_BASE}"
fi

echo "[4/4] Mock service will be stopped automatically."
