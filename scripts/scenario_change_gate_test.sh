#!/usr/bin/env bash
# Tests for scenario_change_gate.sh, scenario_gate_deadline.py,
# scenario_gate_logs.sh and the scenario-gate job's timeout: a hung
# scenario's row must read `timeout` and leave nothing behind, a cancelled
# gate must take its watchdog with it, a log that holds the API key must stop
# the upload, and the job timeout must outlast the script's own limits.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="${REPO_ROOT}/scripts/scenario_change_gate.sh"
LOGS="${REPO_ROOT}/scripts/scenario_gate_logs.sh"
WORKFLOW="${REPO_ROOT}/.github/workflows/scenario-gate.yml"
FAILURES=0
WORK="$(mktemp -d)"
# Nothing of ours outlives the test, whatever it finds.
trap 'pkill -KILL -f "${WORK}" 2> /dev/null; rm -rf "${WORK}"' EXIT
TRIES=50 # polls of 0.1 s: every wait below has this deadline

# check <name> <want> <got>
check() {
  if [[ "$2" == "$3" ]]; then
    echo "  ok    $1"
  else
    echo "  FAIL  $1: expected '$2', got '$3'"
    FAILURES=$((FAILURES + 1))
  fi
}

# wait_for <command...>: polls until the command succeeds; prints ok or timeout.
wait_for() {
  local i
  for ((i = 0; i < TRIES; i++)); do
    "$@" > /dev/null 2>&1 && { echo ok; return; }
    sleep 0.1
  done
  echo timeout
}

# gone <pid>: succeeds once <pid> is no process. An empty pid never succeeds.
gone() { [[ -n "$1" ]] && ! kill -0 "$1" 2> /dev/null; }

# none_left <pattern>: succeeds when no process's command line matches.
none_left() { ! pgrep -f "$1" > /dev/null; }

# scenario_repo <name>: a repo whose HEAD adds scenarios/training/<name>.yaml; sets DIR and BASE.
scenario_repo() {
  DIR="$(mktemp -d "${WORK}/repo.XXXXXX")"
  git -C "${DIR}" init -q
  git -C "${DIR}" config user.email t@t.t
  git -C "${DIR}" config user.name t
  git -C "${DIR}" commit -q --allow-empty -m baseline
  BASE="$(git -C "${DIR}" rev-parse HEAD)"
  mkdir -p "${DIR}/scenarios/training"
  echo "name: $1" > "${DIR}/scenarios/training/$1.yaml"
  git -C "${DIR}" add -A
  git -C "${DIR}" commit -qm scenario
}

# gate_start <name> <binary> <deadline>: the real gate on <name>'s repo, in the
# background, with <binary> in place of infrafactory and a 3 s grace; sets
# GATE_PID. The row lands in $WORK/<name>.tsv.
gate_start() {
  scenario_repo "$1"
  (cd "${DIR}" && exec env INFRAFACTORY_BIN="$2" PER_SCENARIO_TIMEOUT="$3" KILL_AFTER=3 \
    BASE_REF="${BASE}" RESULTS="${WORK}/$1.tsv" LOG_DIR="${WORK}/$1-logs" \
    STUB_CHILD_PID="${WORK}/$1.child" bash "${GATE}" > "${WORK}/$1.out" 2>&1) &
  GATE_PID=$!
}

# gate_status: sets GATE_RC to the started gate's exit code once it is gone,
# or kills it and sets hung. Not a subshell: only this shell can wait for it.
gate_status() {
  if [[ "$(wait_for gone "${GATE_PID}")" != ok ]]; then
    kill -KILL "${GATE_PID}" 2> /dev/null
    GATE_RC=hung
    return
  fi
  wait "${GATE_PID}" 2> /dev/null
  GATE_RC=$?
}

# row <name>: the status and terminal_reason columns of <name>'s row.
row() {
  awk -F'\t' -v n="$1" '$1 == n { print $2 "/" $4 }' "${WORK}/$1.tsv"
}

echo "scenario_change_gate.sh:"

# The hang stub does what a hung infrafactory does: it ignores SIGALRM like
# any Go binary, starts a child in its own process group that ignores TERM
# and records its pid in STUB_CHILD_PID (a tofu that will not stop), and
# sleeps until TERM ends it, leaving the child behind. It ignores TERM only
# until the child has written its pid, so a deadline cannot race the setup:
# a TERM lost there is followed by the KILL, and the row still reads timeout.
STUB="${WORK}/hang"
cat > "${STUB}" <<'STUB'
#!/usr/bin/env perl
use strict;
use warnings;
$SIG{ALRM} = 'IGNORE';
$SIG{TERM} = 'IGNORE';
pipe(my $r, my $w) or die "pipe: $!";
my $pid = fork;
defined $pid or die "fork: $!";
if (!$pid) {
  close $r;
  setpgrp(0, 0) or die "setpgrp: $!";
  $SIG{TERM} = 'IGNORE';
  open my $f, '>', $ENV{STUB_CHILD_PID} or die "open: $!";
  print $f $$;
  close $f;
  close $w;
  sleep 60 while 1;
}
close $w;
<$r>;
$SIG{TERM} = 'DEFAULT';
sleep 60 while 1;
STUB
chmod +x "${STUB}"

gate_start hang "${STUB}" 1
gate_status
check "a hung scenario fails the gate" 1 "${GATE_RC}"
check "a hung scenario's row reads timeout" "timeout/timeout_1s" "$(row hang)"
check "a hung scenario's row is one line" 1 "$(grep -c '^hang' "${WORK}/hang.tsv")"
check "the stub recorded its child" ok "$(wait_for test -s "${WORK}/hang.child")"
if [[ "$(uname)" == Linux ]]; then
  check "the child in its own process group is gone" ok "$(wait_for gone "$(cat "${WORK}/hang.child" 2> /dev/null)")"
  check "nothing of the run is left" ok "$(wait_for none_left "${STUB}")"
else
  echo "  skip  the child in its own process group is gone: no session sweep on $(uname)"
fi
(( FAILURES > 0 )) && cat "${WORK}/hang.out" "${WORK}/hang-logs/hang.log"

gate_start cancel "${STUB}" 60
check "the stub is ready before the cancel" ok "$(wait_for test -s "${WORK}/cancel.child")"
kill -TERM "${GATE_PID}"
gate_status
check "a TERM to the gate ends it by TERM" 143 "${GATE_RC}"
check "the gate's trap stopped its watchdog" 1 "$(grep -c 'scenario-gate-deadline: TERM: stopped' "${WORK}/cancel-logs/cancel.log")"
check "no watchdog outlives the gate" ok "$(wait_for none_left "scenario_gate_deadline.py 60 3 ${STUB}")"
if [[ "$(uname)" == Linux ]]; then
  check "nothing of the run is left after the cancel" ok "$(wait_for none_left "${STUB}")"
fi

# A stopped scenario holds a TERM pending; only the KILL after the grace ends it.
printf '#!/bin/bash\nkill -STOP $$\nsleep 60\n' > "${WORK}/stopped"
chmod +x "${WORK}/stopped"
gate_start stopped "${WORK}/stopped" 1
gate_status
check "a stopped scenario fails the gate" 1 "${GATE_RC}"
check "a stopped scenario's row reads timeout" "timeout/timeout_1s" "$(row stopped)"

printf '#!/bin/bash\necho "Status: success"\necho "- run/terminal_reason: pass (target_reached)"\n' > "${WORK}/ok"
chmod +x "${WORK}/ok"
gate_start ok "${WORK}/ok" 60
gate_status
check "a converged scenario passes the gate" 0 "${GATE_RC}"
check "a converged scenario's row reads success" "success/target_reached" "$(row ok)"

echo "scenario_gate_logs.sh:"

# logs_run <key> <dir>: runs the lister on <dir>'s results.tsv and logs/.
# Prints "<exit code> <stdout joined by ,>".
logs_run() {
  local out rc
  out="$(OPENROUTER_API_KEY="$1" RESULTS="$2/results.tsv" LOG_DIR="$2/logs" bash "${LOGS}" 2> /dev/null | paste -sd, -)"
  rc="${PIPESTATUS[0]}"
  echo "${rc} ${out}"
}

KEY="stub-key-value-for-this-test"
clean="${WORK}/clean"
mkdir -p "${clean}/logs"
printf 'scenario\tstatus\n' > "${clean}/results.tsv"
echo "a ran" > "${clean}/logs/a.log"
echo "b ran" > "${clean}/logs/b.log"
check "clean logs list the TSV and each log by name" \
  "0 ${clean}/results.tsv,${clean}/logs/a.log,${clean}/logs/b.log" "$(logs_run "${KEY}" "${clean}")"

seeded="${WORK}/seeded"
cp -R "${clean}" "${seeded}"
echo "Authorization: Bearer ${KEY} sent" >> "${seeded}/logs/b.log"
check "a log holding the key value lists nothing and fails" "1 " "$(logs_run "${KEY}" "${seeded}")"
check "an unset key lists nothing and fails" "1 " "$(logs_run "" "${clean}")"
check "no files lists nothing and passes" "0 " "$(logs_run "${KEY}" "${WORK}/absent")"

echo "scenario-gate.yml:"

got="$(python3 - "${WORKFLOW}" <<'PY'
import sys, yaml
job = yaml.safe_load(open(sys.argv[1]))["jobs"]["scenario-gate"]
limit = int(job["env"]["MAX_SCENARIOS"]) * int(job["env"]["PER_SCENARIO_TIMEOUT"]) / 60
print("ok" if job["timeout-minutes"] > limit else f"timeout-minutes {job['timeout-minutes']} <= {limit:g}")
PY
)"
check "the job timeout exceeds MAX_SCENARIOS x PER_SCENARIO_TIMEOUT / 60" ok "${got}"

if (( FAILURES > 0 )); then
  echo "${FAILURES} case(s) failed"
  exit 1
fi
echo "all cases passed"
