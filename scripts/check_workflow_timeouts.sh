#!/usr/bin/env bash
# Usage: check_workflow_timeouts.sh [workflow-dir]
#
# Fails when a job in .github/workflows/*.y{a,}ml has no timeout-minutes, so a
# hung job cannot sit on GitHub's 6-hour default. Passes only when every file
# parses, has a jobs mapping, and every job sets timeout-minutes to a positive
# number or a ${{ }} expression; a missing directory, a parse error or a
# missing PyYAML fails too. A job that calls a reusable workflow (job-level
# uses:) is skipped, since GitHub rejects timeout-minutes there. A local
# ./.github/workflows/ callee is checked through its own file; a remote
# owner/repo/...@ref callee is not checked at all.
set -euo pipefail

dir="${1:-.github/workflows}"

python3 - "${dir}" <<'PY'
import glob, re, sys
import yaml


def valid(timeout):
    if isinstance(timeout, bool):
        return False
    if isinstance(timeout, (int, float)):
        return timeout > 0
    return isinstance(timeout, str) and re.fullmatch(r"\$\{\{.+\}\}", timeout.strip()) is not None

files = sorted(glob.glob(f"{sys.argv[1]}/*.yml") + glob.glob(f"{sys.argv[1]}/*.yaml"))
if not files:
    sys.exit(f"no workflow files under {sys.argv[1]}")

missing, skipped = [], []
for path in files:
    with open(path) as f:
        jobs = (yaml.safe_load(f) or {}).get("jobs")
    if not isinstance(jobs, dict) or not jobs:
        sys.exit(f"{path}: no jobs mapping")
    for name, job in jobs.items():
        if isinstance(job, dict) and "uses" in job:
            skipped.append(f"{name} ({job['uses']})")
            continue
        timeout = job.get("timeout-minutes") if isinstance(job, dict) else None
        if not valid(timeout):
            missing.append(f"{path}: job '{name}' has no valid timeout-minutes (got {timeout!r})")

if missing:
    print("\n".join(missing))
    sys.exit(1)
if skipped:
    print(f"workflow timeouts: skipped reusable-workflow calls {', '.join(skipped)}; every other job in {len(files)} files sets timeout-minutes")
else:
    print(f"workflow timeouts: every job in {len(files)} files sets timeout-minutes")
PY
