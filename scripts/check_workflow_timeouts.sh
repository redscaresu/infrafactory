#!/usr/bin/env bash
# Usage: check_workflow_timeouts.sh [workflow-dir]
#
# Fails when a job in .github/workflows/*.y{a,}ml has no timeout-minutes, so a
# hung job cannot sit on GitHub's 6-hour default. Passes only when every file
# parses, has a jobs mapping, and every job sets timeout-minutes; a missing
# directory, a parse error or a missing PyYAML fails too.
set -euo pipefail

dir="${1:-.github/workflows}"

python3 - "${dir}" <<'PY'
import glob, sys
import yaml

files = sorted(glob.glob(f"{sys.argv[1]}/*.yml") + glob.glob(f"{sys.argv[1]}/*.yaml"))
if not files:
    sys.exit(f"no workflow files under {sys.argv[1]}")

missing = []
for path in files:
    with open(path) as f:
        jobs = (yaml.safe_load(f) or {}).get("jobs")
    if not isinstance(jobs, dict) or not jobs:
        sys.exit(f"{path}: no jobs mapping")
    for name, job in jobs.items():
        if not isinstance(job, dict) or "timeout-minutes" not in job:
            missing.append(f"{path}: job '{name}' has no timeout-minutes")

if missing:
    print("\n".join(missing))
    sys.exit(1)
print(f"workflow timeouts: every job in {len(files)} files sets timeout-minutes")
PY
