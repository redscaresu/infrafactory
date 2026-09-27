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

# Dependency manifests are exempt from the STATUS.md rule -- but only when
# they are the ONLY thing that changed.
#
# The rule exists so a human records what a code change means. A dependabot
# bump has no such meaning to record, and dependabot cannot edit STATUS.md,
# so requiring it made every Go dependency PR permanently red -- and with
# required status checks on main, permanently unmergeable. Eight of them had
# piled up, the oldest three weeks old, which is the opposite of the
# "keep dependencies updated" posture this repo wants.
#
# The exemption is deliberately all-or-nothing: a PR that bumps go.mod AND
# edits internal/ still needs STATUS.md, so a real change cannot smuggle
# itself in behind a lockfile.
DEPENDENCY_MANIFESTS=("go.mod" "go.sum" "ui/package.json" "ui/package-lock.json")

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

only_dependency_manifests() {
  for f in "${CHANGED[@]}"; do
    local matched=false
    for manifest in "${DEPENDENCY_MANIFESTS[@]}"; do
      if [[ "${f}" == "${manifest}" ]]; then
        matched=true
        break
      fi
    done
    if [[ "${matched}" == "false" ]]; then
      return 1
    fi
  done
  return 0
}

if [[ "${status_required}" == "true" ]] && only_dependency_manifests; then
  echo "Dependency-manifest-only change: STATUS.md not required."
  status_required=false
fi

# STATUS.md is the entry point every session reads, so its size is a cost paid
# on every session. It holds current state only (ADR-0035); history lives in
# docs/status/. The cap forces pruning instead of letting it grow back.
STATUS_MAX_LINES=150
if [[ -f STATUS.md ]] && (( $(wc -l < STATUS.md) > STATUS_MAX_LINES )); then
  echo "Doc hygiene check failed: STATUS.md is over ${STATUS_MAX_LINES} lines. Drop the oldest Recent lines; history belongs in docs/status/."
  exit 1
fi

if [[ "${status_required}" == "true" ]] && ! contains_file "STATUS.md"; then
  echo "Doc hygiene check failed: code/config changes require STATUS.md update."
  exit 1
fi

# A decision-impacting PATH does not always carry a decision: repointing a
# comment in internal/cli changes no contract, and forcing an ADR edit for it
# turned ADR amendments into changelogs (ADR-0035). The trailer keeps the
# forcing function -- someone classifies the change and says why -- without
# the edit.
#
# The trailer must be on the LATEST commit of the range, so it is declared with
# the whole change in view. Honouring it anywhere in the range let a trailer on
# an early comment-only commit waive a later contract change; checking each
# commit instead missed decision-path edits carried by merge commits. A tip
# that does not declare it -- a merge commit included -- fails closed. Read
# from the commit message, so it is honoured in CI; in --staged mode there is
# no message yet, so it can only be a note there.
adr_declared_none() {
  [[ -n "${HEAD_SHA}" ]] || return 1
  git log -1 --format=%B "${HEAD_SHA}" | grep -qE '^ADR: none ?(—|--|-) ?.{10,}'
}

if [[ "${decision_required}" == "true" ]]; then
  if ! any_changed_matching_regex '^docs/decisions/[0-9]{4}-.*\.md$'; then
    if adr_declared_none; then
      echo "Decision-impacting paths changed; declared implementation-only by an 'ADR: none' trailer."
    elif [[ "${MODE}" == "--staged" ]]; then
      echo "Note: decision-impacting paths changed and no ADR is staged. Add one, or put 'ADR: none — <reason>' in the commit message; CI enforces it on the PR."
    else
      echo "Doc hygiene check failed: decision-impacting changes require an ADR update in docs/decisions/NNNN-title.md, or an 'ADR: none — <reason>' trailer in a commit message."
      exit 1
    fi
  fi
fi

if [[ "${#NEW_ADRS[@]}" -gt 0 ]] && ! contains_file "docs/decisions/README.md"; then
  echo "Doc hygiene check failed: new ADRs require docs/decisions/README.md index update."
  exit 1
fi

echo "Doc hygiene checks passed."
