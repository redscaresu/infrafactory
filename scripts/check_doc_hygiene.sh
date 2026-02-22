#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-}"
BASE_SHA=""
HEAD_SHA=""
CHANGED=()
NEW_ADRS=()

if [[ "${MODE}" == "--staged" ]]; then
  while IFS= read -r line; do
    CHANGED+=("${line}")
  done < <(git diff --cached --name-only)

  while IFS= read -r line; do
    NEW_ADRS+=("${line}")
  done < <(git diff --cached --name-status | awk '$1=="A" && $2 ~ /^docs\/decisions\/[0-9]{4}-.*\.md$/ {print $2}')
else
  BASE_SHA="${1:-}"
  HEAD_SHA="${2:-}"

  if [[ -z "${BASE_SHA}" || -z "${HEAD_SHA}" ]]; then
    echo "usage:"
    echo "  $0 <base-sha> <head-sha>"
    echo "  $0 --staged"
    exit 2
  fi

  if ! git rev-parse --verify "${BASE_SHA}^{commit}" >/dev/null 2>&1; then
    echo "Invalid base SHA: ${BASE_SHA}"
    exit 2
  fi
  if ! git rev-parse --verify "${HEAD_SHA}^{commit}" >/dev/null 2>&1; then
    echo "Invalid head SHA: ${HEAD_SHA}"
    exit 2
  fi

  while IFS= read -r line; do
    CHANGED+=("${line}")
  done < <(git diff --name-only "${BASE_SHA}" "${HEAD_SHA}")

  while IFS= read -r line; do
    NEW_ADRS+=("${line}")
  done < <(git diff --name-status "${BASE_SHA}" "${HEAD_SHA}" | awk '$1=="A" && $2 ~ /^docs\/decisions\/[0-9]{4}-.*\.md$/ {print $2}')
fi

if [[ "${#CHANGED[@]}" -eq 0 ]]; then
  echo "No changed files."
  exit 0
fi

contains_file() {
  local target="$1"
  for f in "${CHANGED[@]}"; do
    if [[ "${f}" == "${target}" ]]; then
      return 0
    fi
  done
  return 1
}

matches_any_prefix() {
  local f="$1"
  shift
  local prefixes=("$@")
  for p in "${prefixes[@]}"; do
    if [[ "${f}" == "${p}"* ]]; then
      return 0
    fi
  done
  return 1
}

any_changed_in_prefixes() {
  local prefixes=("$@")
  for f in "${CHANGED[@]}"; do
    if matches_any_prefix "${f}" "${prefixes[@]}"; then
      return 0
    fi
  done
  return 1
}

any_changed_matching_regex() {
  local re="$1"
  for f in "${CHANGED[@]}"; do
    if [[ "${f}" =~ ${re} ]]; then
      return 0
    fi
  done
  return 1
}

STATUS_REQUIRED_PREFIXES=("cmd/" "internal/" "prompts/" "policies/" "scenarios/" "testdata/")
STATUS_REQUIRED_FILES=("go.mod" "go.sum" "scenario.schema.json" "infrafactory.yaml")

DECISION_PREFIXES=("cmd/infrafactory/" "internal/cli/")
DECISION_FILES=("scenario.schema.json" "infrafactory.yaml" "docs/architecture.md")

status_required=false
decision_required=false

if any_changed_in_prefixes "${STATUS_REQUIRED_PREFIXES[@]}"; then
  status_required=true
fi
for f in "${STATUS_REQUIRED_FILES[@]}"; do
  if contains_file "${f}"; then
    status_required=true
  fi
done

for f in "${DECISION_FILES[@]}"; do
  if contains_file "${f}"; then
    decision_required=true
  fi
done

# A CLI contract change is likely when CLI entrypoints/command wiring changes.
if any_changed_in_prefixes "${DECISION_PREFIXES[@]}"; then
  for f in "${CHANGED[@]}"; do
    if [[ "${f}" == cmd/infrafactory/* ]] || [[ "${f}" == internal/cli/* ]]; then
      decision_required=true
    fi
  done
fi

if [[ "${status_required}" == "true" ]] && ! contains_file "STATUS.md"; then
  echo "Doc hygiene check failed: code/config changes require STATUS.md update."
  exit 1
fi

if [[ "${decision_required}" == "true" ]]; then
  if ! any_changed_matching_regex '^docs/decisions/[0-9]{4}-.*\.md$'; then
    echo "Doc hygiene check failed: decision-impacting changes require ADR update in docs/decisions/NNNN-title.md."
    exit 1
  fi
fi

if [[ "${#NEW_ADRS[@]}" -gt 0 ]] && ! contains_file "docs/decisions/README.md"; then
  echo "Doc hygiene check failed: new ADRs require docs/decisions/README.md index update."
  exit 1
fi

echo "Doc hygiene checks passed."
