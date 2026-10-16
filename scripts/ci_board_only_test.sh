#!/usr/bin/env bash
# Tests for ci_board_only.sh.
#
# A wrong "skip" turns a PR's required checks green without running them, so
# every case except the all-board one must come out "run".
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/ci_board_only.sh"
FAILURES=0

# new_repo: a throwaway repo with one baseline commit; prints its path.
new_repo() {
  local dir
  dir="$(mktemp -d)"
  git -C "${dir}" init -q
  git -C "${dir}" config user.email t@t.t
  git -C "${dir}" config user.name t
  mkdir -p "${dir}/internal" "${dir}/docs/stories"
  echo "package x" > "${dir}/internal/x.go"
  echo "story" > "${dir}/docs/stories/a.md"
  git -C "${dir}" add -A
  git -C "${dir}" commit -qm baseline
  echo "${dir}"
}

# commit_all <dir>: commits whatever changed (possibly nothing); prints HEAD.
commit_all() {
  git -C "$1" add -A
  git -C "$1" commit -qm change --allow-empty
  git -C "$1" rev-parse HEAD
}

# expect <name> <skip|run> <dir> <base> <head>: runs the real script in <dir>.
expect() {
  local name="$1" want="$2" dir="$3" got="run"
  (cd "${dir}" && bash "${SCRIPT}" "$4" "$5" > /dev/null 2>&1) && got="skip"
  rm -rf "${dir}"
  if [[ "${got}" == "${want}" ]]; then
    echo "  ok    ${name}"
  else
    echo "  FAIL  ${name}: expected ${want}, got ${got}"
    FAILURES=$((FAILURES + 1))
  fi
}

# change_case <name> <skip|run> <file>...: one commit editing each file.
change_case() {
  local name="$1" want="$2" dir base
  shift 2
  dir="$(new_repo)"
  base="$(git -C "${dir}" rev-parse HEAD)"
  for f in "$@"; do
    mkdir -p "${dir}/$(dirname "${f}")"
    echo "change" >> "${dir}/${f}"
  done
  expect "${name}" "${want}" "${dir}" "${base}" "$(commit_all "${dir}")"
}

echo "ci_board_only.sh:"

change_case "every board path gives skip" skip \
  docs/stories/b.md docs/epics/e.md docs/hld/h.md docs/Board.base STATUS.md
change_case "one non-board file gives run" run docs/stories/b.md internal/x.go
change_case "a board name outside the board gives run" run internal/STATUS.md
change_case "an empty list gives run" run

dir="$(new_repo)"
expect "a failed git diff gives run" run "${dir}" \
  0000000000000000000000000000000000000000 "$(git -C "${dir}" rev-parse HEAD)"

dir="$(new_repo)"
expect "a missing argument gives run" run "${dir}" "" "$(git -C "${dir}" rev-parse HEAD)"

dir="$(new_repo)"
base="$(git -C "${dir}" rev-parse HEAD)"
git -C "${dir}" mv internal/x.go docs/stories/x.go
expect "a rename into docs/stories/ gives run" run "${dir}" "${base}" "$(commit_all "${dir}")"

if (( FAILURES > 0 )); then
  echo "${FAILURES} case(s) failed"
  exit 1
fi
echo "all cases passed"
