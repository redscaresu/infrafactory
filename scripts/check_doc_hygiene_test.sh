#!/usr/bin/env bash
# Tests for check_doc_hygiene.sh.
#
# The dependency carve-out relaxes a rule that exists to keep code changes
# documented, so it needs a test proving it cannot be used as a bypass: a
# bump that also touches real code must still be rejected.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/check_doc_hygiene.sh"
FAILURES=0

# run_case <name> <expected: pass|fail> <file>...
# Builds a throwaway repo with one baseline commit and one commit touching
# the named files, then runs the real check across that range.
run_case() {
  local name="$1" expected="$2"
  shift 2

  local dir
  dir="$(mktemp -d)"
  (
    cd "${dir}" || exit 1
    git init -q .
    git config user.email t@t.t
    git config user.name t
    mkdir -p cmd/infrafactory internal/cli internal/harness docs/decisions ui
    echo baseline > README.md
    git add -A && git commit -qm baseline
    local base
    base="$(git rev-parse HEAD)"

    for f in "$@"; do
      mkdir -p "$(dirname "${f}")"
      echo "change $(date +%s%N)" >> "${f}"
    done
    git add -A && git commit -qm "${MSG:-change}"
    local head
    head="$(git rev-parse HEAD)"

    bash "${SCRIPT}" "${base}" "${head}" > /dev/null 2>&1
  )
  local rc=$?
  rm -rf "${dir}"

  local actual="pass"
  [[ ${rc} -ne 0 ]] && actual="fail"

  if [[ "${actual}" == "${expected}" ]]; then
    echo "  ok    ${name}"
  else
    echo "  FAIL  ${name}: expected ${expected}, got ${actual} (exit ${rc})"
    FAILURES=$((FAILURES + 1))
  fi
}


# run_two_commits <name> <expected> <msg1> <file1> <msg2> <file2>
# The trailer counts only on the latest commit: an early declaration must not
# waive a later, undeclared change.
run_two_commits() {
  local name="$1" expected="$2" msg1="$3" f1="$4" msg2="$5" f2="$6" dir rc actual
  dir="$(mktemp -d)"
  (
    cd "${dir}" || exit 1
    git init -q . && git config user.email t@t.t && git config user.name t
    echo baseline > README.md && git add -A && git commit -qm baseline
    base="$(git rev-parse HEAD)"
    mkdir -p "$(dirname "${f1}")" && echo a >> "${f1}" && echo a >> STATUS.md && git add -A && git commit -qm "${msg1}"
    mkdir -p "$(dirname "${f2}")" && echo b >> "${f2}" && git add -A && git commit -qm "${msg2}"
    bash "${SCRIPT}" "${base}" "$(git rev-parse HEAD)" > /dev/null 2>&1
  )
  rc=$?; rm -rf "${dir}"
  actual="pass"; [[ ${rc} -ne 0 ]] && actual="fail"
  if [[ "${actual}" == "${expected}" ]]; then echo "  ok    ${name}"
  else echo "  FAIL  ${name}: expected ${expected}, got ${actual}"; FAILURES=$((FAILURES + 1)); fi
}

echo "check_doc_hygiene.sh"

# The carve-out itself. Dependabot cannot edit STATUS.md, and a lockfile bump
# has no meaning to record, so these must not be blocked.
run_case "go.mod+go.sum only is exempt"            pass go.mod go.sum
run_case "ui lockfile only is exempt"              pass ui/package.json ui/package-lock.json
run_case "all manifests together are exempt"       pass go.mod go.sum ui/package.json ui/package-lock.json

# The carve-out must not become a bypass.
run_case "go.mod + internal code still needs STATUS"     fail go.mod internal/harness/x.go
run_case "lockfile + internal code still needs STATUS"   fail ui/package-lock.json internal/harness/x.go

# Pre-existing behaviour must be unchanged.
run_case "internal change without STATUS is rejected"    fail internal/harness/x.go
run_case "internal change with STATUS is accepted"       pass internal/harness/x.go STATUS.md
run_case "internal/cli needs an ADR too"                 fail internal/cli/x.go STATUS.md
MSG="ADR: none — comment-only, repoints a doc link" \
  run_case "internal/cli with an ADR: none trailer is accepted" pass internal/cli/x.go STATUS.md
MSG="ADR: none" \
  run_case "an ADR: none trailer needs a reason"             fail internal/cli/x.go STATUS.md
MSG="ADR: none — ok" \
  run_case "a token reason is not a reason"                  fail internal/cli/x.go STATUS.md
MSG="ADR: none — comment-only, repoints a doc link" \
  run_case "the trailer does not waive STATUS.md"            fail internal/cli/x.go
run_two_commits "one commit's trailer does not waive another commit" fail \
  "ADR: none — comment-only, repoints a doc link" internal/cli/a.go \
  "change the CLI contract" internal/cli/b.go
run_two_commits "a declaration on the latest commit covers the range" pass \
  "tidy a comment" internal/cli/a.go \
  "ADR: none — comment-only and test-only, no contract change" internal/cli/b.go
# A *new* ADR must also be indexed; amending an existing one need not be.
run_case "new ADR without README index is rejected"      fail internal/cli/x.go STATUS.md docs/decisions/0001-x.md
run_case "new ADR with README index is accepted"         pass internal/cli/x.go STATUS.md docs/decisions/0001-x.md docs/decisions/README.md
run_case "docs-only change is accepted"                  pass docs/whatever.md

if [[ ${FAILURES} -gt 0 ]]; then
  echo "${FAILURES} case(s) failed."
  exit 1
fi
echo "All doc-hygiene cases passed."
