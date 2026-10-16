#!/usr/bin/env bash
# Usage: ci_board_only.sh <base-sha> <head-sha>
#
# Decides whether a pull request changes only board files: a .md file under
# docs/stories/, docs/epics/ or docs/hld/, or docs/Board.base or STATUS.md.
# CI skips its full jobs only when this exits 0. Every other outcome (a
# non-board file, an empty list, a bad argument, a failed diff, a crash)
# exits non-zero, and they run.
set -uo pipefail

base="${1:-}"
head="${2:-}"

run_full() {
  echo "board only: no ($1): running the full job"
  exit 1
}

for sha in "${base}" "${head}"; do
  [[ "${sha}" =~ ^[0-9a-f]{40}$ ]] || run_full "'${sha}' is not a commit SHA"
done

# --no-renames lists a rename's old path as well as its new one, so moving a
# file into docs/stories/ still counts the file it deleted.
files="$(git diff --name-only --no-renames "${base}...${head}" --)" \
  || run_full "git diff ${base}...${head} failed"
[[ -n "${files}" ]] || run_full "the diff lists no files"

echo "Changed files:"
echo "${files}"

while IFS= read -r file; do
  case "${file}" in
    docs/stories/*.md | docs/epics/*.md | docs/hld/*.md | docs/Board.base | STATUS.md) ;;
    *) ;;
  esac
done <<< "${files}"

echo "board only: skipped (every changed file is a board file)"
exit 0
