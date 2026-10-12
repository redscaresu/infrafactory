#!/usr/bin/env bash
# Runs the Playwright suite (arguments pass through to `playwright test`)
# and fails if the test server logged a run-mode error.
#
# Every page answers run-mode from ui/e2e/fixtures.ts, because the suite's
# server has no mock behind it. A "could not determine the run mode" line
# means a page reached the real handler anyway: noise that buries real
# server errors, and a sign a spec bypassed the fixture.
set -uo pipefail

cd "$(dirname "$0")/../ui" || exit 1
log=$(mktemp) || exit 1
trap 'rm -f "$log"' EXIT

npx playwright test "$@" 2>&1 | tee "$log"
status=${PIPESTATUS[0]}

grep -n '\[WebServer\].*could not determine the run mode' "$log"
case $? in
  1) ;;
  0) echo "FAIL: the test server logged a run-mode error (lines above); route run-mode through ui/e2e/fixtures.ts" >&2; exit 1 ;;
  *) echo "FAIL: could not read the Playwright log $log" >&2; exit 1 ;;
esac
exit "$status"
