#!/usr/bin/env bash
# swarm.sh — run agents in herdr panes so every one can be watched.
#
#   swarm.sh agent <tab> <name> <cwd> <role> <prompt-file>   start one agent in its own pane
#   swarm.sh story <slug>                                   build one docs/stories/<slug>.md
#   swarm.sh conduct <epic>                                 a fresh conductor that drives one epic's stories to merge
#   swarm.sh wait <name> [timeout-ms]                       block until the agent settles
#   swarm.sh policy <role>                                  print the model and effort for a role
#   swarm.sh close <tab>                                    close <tab>, <tab>-2, ... and forget them
#   swarm.sh unblock                                        mark ready every blocked story whose blockers are all merged
#   swarm.sh watch [repo...]                                wait until a story PR needs the lead, print why, and exit
#
# Must run inside herdr (HERDR_ENV=1). Tabs hold at most four panes (a 2x2 grid); a fifth
# agent opens "<tab>-2", and so on, so no pane gets too small to follow.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${REPO_ROOT}/.swarm/state"
PANES_PER_TAB=4

die() { echo "swarm: $*" >&2; exit 1; }

# The model and effort for each role. This is the one place the policy lives
# (docs/operations.md § Model and effort explains the reasoning).
#   design:    hld, hld-review, hld-lead
#   planning:  survey, lead, skeptic, critic, codex
#   building:  code, code-risky, docs, chore, verify
#   conducting: conduct
policy() {
  case "$1" in
    hld)        echo "claude fable high" ;;    # co-writes the HLD with the user, who waits on every turn
    hld-review) echo "claude fable xhigh" ;;   # attacks the finished draft before it is agreed
    hld-lead)   echo "claude fable xhigh" ;;   # HLD into epics: one call that shapes everything below it
    survey)     echo "claude sonnet high" ;;   # reads and cites; feeds the lead, so not low
    lead)       echo "claude opus xhigh" ;;    # one decomposition decides the whole epic
    skeptic)    echo "claude opus high" ;;     # the only gate a story passes before it is built
    critic)     echo "claude opus high" ;;
    codex)      echo "codex default high" ;;   # a different model family, read-only
    code)       echo "claude opus high" ;;
    code-risky) echo "claude opus xhigh" ;;    # Layer 3, teardown, safety or hygiene paths
    docs|chore) echo "claude sonnet medium" ;;
    verify)     echo "claude sonnet medium" ;; # run tests or commands and report
    escalate)   echo "claude fable xhigh" ;;   # only after a story failed twice, or an unreconcilable epic
    conduct)    echo "claude opus high" ;;     # dispatches, reviews, merges one epic; a fresh session each time
    *) die "unknown role '$1' (hld hld-review hld-lead survey lead skeptic critic codex code code-risky docs chore verify escalate conduct)" ;;
  esac
}

# agent_name <name> — the herdr agent name for anything started or waited on. start_agent and
# `wait` both map through here, so every caller agrees: lowercase, [a-z0-9_-], starting with a letter, at most 32 characters.
agent_name() {
  local n
  n=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_-' '-')
  [[ "${n}" =~ ^[a-z] ]] || n="s${n}"
  printf '%s' "${n:0:32}"
}

json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }

require_herdr() { [[ "${HERDR_ENV:-}" == 1 ]] || die "not running inside a herdr pane (HERDR_ENV != 1)"; }

# next_pane <tab-label> <cwd> — a fresh shell pane in a tab with room, creating tabs as needed.
next_pane() {
  local label="$1" cwd="$2" n=1 state panes count pane
  mkdir -p "${STATE_DIR}"
  while :; do
    local tab_label="${label}"; [[ ${n} -gt 1 ]] && tab_label="${label}-${n}"
    state="${STATE_DIR}/${tab_label}"
    count=0; [[ -f "${state}" ]] && count=$(wc -l < "${state}" | tr -d ' ')
    if [[ ${count} -lt ${PANES_PER_TAB} ]]; then
      if [[ ${count} -eq 0 ]]; then
        pane=$(herdr tab create --workspace "${HERDR_WORKSPACE_ID}" --cwd "${cwd}" --label "${tab_label}" --no-focus \
          | json "d['result']['root_pane']['pane_id']")
      else
        # 2x2: 2nd splits the first to the right, 3rd splits the first down, 4th splits the second down.
        panes=($(cat "${state}"))
        case ${count} in
          1) pane=$(herdr pane split "${panes[0]}" --direction right --cwd "${cwd}" --no-focus | json "d['result']['pane']['pane_id']") ;;
          2) pane=$(herdr pane split "${panes[0]}" --direction down  --cwd "${cwd}" --no-focus | json "d['result']['pane']['pane_id']") ;;
          3) pane=$(herdr pane split "${panes[1]}" --direction down  --cwd "${cwd}" --no-focus | json "d['result']['pane']['pane_id']") ;;
        esac
      fi
      echo "${pane}" >> "${state}"
      echo "${pane}"
      return
    fi
    n=$((n + 1))
  done
}

start_agent() {
  local tab="$1" name cwd="$3" role="$4" prompt_file="$5" kind model effort pane
  name="$(agent_name "$2")"   # the one place names are mapped, so `wait <same name>` always finds it
  [[ -f "${prompt_file}" ]] || die "no prompt file ${prompt_file}"
  read -r kind model effort <<< "$(policy "${role}")"
  pane=$(next_pane "${tab}" "${cwd}")
  # A freshly split pane is not always an available shell yet (agent_pane_busy): retry once.
  local try
  for try in 1 2; do
    if [[ "${kind}" == codex ]]; then
      herdr agent start "${name}" --kind codex --pane "${pane}" --timeout 60000 -- \
        -s read-only -c "model_reasoning_effort=\"${effort}\"" >/dev/null && break
    else
      herdr agent start "${name}" --kind claude --pane "${pane}" --timeout 60000 -- \
        --model "${model}" --effort "${effort}" --permission-mode auto >/dev/null && break
    fi
    [[ ${try} -eq 2 ]] && die "${name}: herdr could not start the agent in pane ${pane}"
    sleep 3
  done
  deliver_prompt "${name}" "${prompt_file}"
  echo "${name} ${pane} ${kind}:${model}:${effort}"
}

# deliver_prompt <name> <prompt-file> — submit the brief and confirm the agent acted on it.
# `agent start` returns once the agent is ready, but a slow starter (Fable at xhigh) could
# still drop a prompt sent at once, leaving it idle at an empty prompt while every later
# `wait` returned immediately. So the submission waits until herdr sees the agent working
# (or blocked on a question), retries once, and fails loudly rather than reporting success.
deliver_prompt() {
  local name="$1" prompt_file="$2" attempt status
  for attempt in 1 2; do
    status=$(herdr agent prompt "${name}" "$(cat "${prompt_file}")" --wait --until working --until blocked \
      --timeout 60000 2>/dev/null | json "d.get('result',{}).get('agent',{}).get('agent_status','')" 2>/dev/null || true)
    case "${status}" in
      working|blocked) return 0 ;;
    esac
    sleep 5
  done
  die "${name}: the brief was not taken up after 2 attempts (status '${status:-none}'); see herdr agent read ${name}"
}

# The brief every story builder gets. The story file is the task; these are the rules.
story_brief() {
  local slug="$1" repo="$2" story_ref cleanup
  story_ref="docs/stories/${slug}.md"
  cleanup="Delete docs/stories/${slug}.md in your PR, and nothing else under
docs/stories/."
  if [[ -n "${repo}" ]]; then
    # The story lives in infrafactory, the work in a sibling repo: read it there, leave it for the lead.
    story_ref="${REPO_ROOT}/docs/stories/${slug}.md (this worktree is ../${repo})"
    cleanup="Do not touch the story file; the lead deletes it after your PR merges."
  fi
  cat <<EOF
Implement ${story_ref}. Its **Done when** is the acceptance.

Title your commit and PR with a plain description of the change, and no slice number: slice
numbers (S###) belong to the lead, and two agents choosing one produce duplicates in the log.

Rules: read AGENTS.md first. ${cleanup} Never merge (the lead merges). Never touch real cloud or credentials: do not source
~/.config/infrafactory/*.env, and do not run deploy or anything with sandbox_deploy enabled. Run
\`codex exec review --base main\` before committing; fix real findings, decline nits with a reason,
converge on one clean pass, and record the loop in the PR body. If codex reports a usage limit, do
not wait for it to reset: carry on without it and write "codex skipped: usage limit" in the PR body. If you touch internal/cli without a
real decision, put \`ADR: none — <reason>\` on your final commit. End commit messages with
"Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" and PR bodies with
"🤖 Generated with [Claude Code](https://claude.com/claude-code)". If \`gh pr view --json mergeable\` says
CONFLICTING, merge origin/main into your branch, resolve it (keep both sides of any list), re-run
the tests and push: a conflicted PR runs no checks and waits forever. The merge commit becomes the
tip, and doc hygiene reads the ADR trailer from the tip only, so repeat your \`ADR: none — <reason>\`
line in the merge commit's message. When CI is green, reply with the
PR URL, what changed in three lines, and the codex findings, then stop.
EOF
}

build_story() {
  local slug="$1" story="${REPO_ROOT}/docs/stories/$1.md" kind risk repo src role wt branch prompt
  [[ -f "${story}" ]] || die "no story ${story}"
  grep -q '^status: ready$' "${story}" || die "${slug} is not status: ready"
  kind=$(sed -n 's/^kind: *//p' "${story}" | head -1); kind="${kind:-code}"
  case "${kind}" in
    lead)     die "${slug} is kind: lead — real cloud or credentials; the lead runs it, not a swarm agent" ;;
    operator) die "${slug} is kind: operator — a human step, not an agent's" ;;
  esac
  risk=$(sed -n 's/^risk: *//p' "${story}" | head -1)
  repo=$(sed -n 's/^repo: *//p' "${story}" | head -1)   # a sibling repo (fakeaws, mockway, ...); empty is infrafactory
  role="${kind}"; [[ "${kind}" == code && "${risk}" == high ]] && role=code-risky
  policy "${role}" >/dev/null
  src="${REPO_ROOT}"
  if [[ -n "${repo}" ]]; then
    [[ "${repo}" =~ ^[a-z0-9-]+$ ]] || die "${slug}: bad repo '${repo}'"
    src="$(dirname "${REPO_ROOT}")/${repo}"
    [[ -e "${src}/.git" ]] || die "${slug}: no repo at ${src}"
  fi
  branch="story/${slug}"; wt="$(dirname "${REPO_ROOT}")/$(basename "${src}")-wt/${slug}"
  git -C "${src}" fetch -q origin main
  git -C "${src}" worktree add -q -b "${branch}" "${wt}" origin/main
  prompt="${REPO_ROOT}/.swarm/briefs/${slug}.md"; mkdir -p "$(dirname "${prompt}")"
  story_brief "${slug}" "${repo}" > "${prompt}"
  start_agent "build" "${slug}" "${wt}" "${role}" "${prompt}"
}

# The brief a conductor gets: one epic, from a fresh session, so its context holds only that epic.
conduct_brief() {
  local epic="$1"
  cat <<EOF
You are the conductor for the epic docs/epics/${epic}.md. Read AGENTS.md, STATUS.md,
docs/operations.md and the epic first.

Drive the epic's stories (docs/stories/*.md with \`epic: ${epic}\`) to merge, as docs/operations.md
§ Parallel agents describes: pick waves of \`ready\` stories whose \`touches\` do not overlap,
start each with \`scripts/swarm.sh story <slug>\`, wait with \`scripts/swarm.sh wait\` and
\`scripts/swarm.sh watch\` in the background, review each PR, merge it only when its head is green,
then run \`scripts/swarm.sh unblock\`, and \`scripts/swarm.sh close build\` once a wave is done.

Stay inside this epic: start no story outside it, and leave the HLD and other epics alone. A
\`kind: lead\` story (real cloud, credentials) or a \`kind: operator\` one is not yours to run; list it
for the user. Stop when the epic's **Done when** holds (delete the epic file in the last PR) or
when nothing ready is left, and reply with what merged, what is left, and what waits on the user.
EOF
}

start_conductor() {
  local epic="$1" prompt
  [[ "${epic}" =~ ^[a-z0-9-]+$ ]] || die "bad epic '${epic}'"
  [[ -f "${REPO_ROOT}/docs/epics/${epic}.md" ]] || die "no epic docs/epics/${epic}.md"
  prompt="${REPO_ROOT}/.swarm/briefs/conduct-${epic}.md"; mkdir -p "$(dirname "${prompt}")"
  conduct_brief "${epic}" > "${prompt}"
  start_agent "conduct" "conduct-${epic}" "${REPO_ROOT}" conduct "${prompt}"
}

# unblock — a merged story's file is deleted, so a blocked story whose every blocked_by slug has no
# story or epic file left, and names no operator step, is ready. Prints each story it flips.
unblock() {
  local story slug blockers b open
  for story in "${REPO_ROOT}"/docs/stories/*.md; do
    grep -q '^status: blocked$' "${story}" || continue
    blockers=$(sed -n 's/^blocked_by: *\[\(.*\)\]$/\1/p' "${story}" | tr ',' ' ')
    [[ -n "${blockers// /}" ]] || continue   # blocked for a reason no merge clears
    open=0
    for b in ${blockers}; do
      # Open while its story exists, its epic exists, or it is not a plain slug (an operator step such
      # as "operator:planted-leak-proof" is cleared by hand, never by a merge).
      if [[ -f "${REPO_ROOT}/docs/stories/${b}.md" || -f "${REPO_ROOT}/docs/epics/${b}.md" || ! "${b}" =~ ^[a-z0-9-]+$ ]]; then open=1; fi
    done
    [[ ${open} -eq 0 ]] || continue
    sed -i.bak -e 's/^status: blocked$/status: ready/' -e '/^blocked_by:/d' "${story}" && rm -f "${story}.bak"
    echo "ready: $(basename "${story}" .md)"
  done
}

# watch [repo...] — block until a story/* PR in these repos needs the lead, print one line saying
# which and why, and exit 0. A PR needs the lead when its checks on a new head have all finished,
# when it conflicts with main (a conflicted PR runs no checks, so waiting on checks never ends),
# or when its head has had no check at all for WATCH_STALL_SECS (default 600). A herdr agent
# blocked on a prompt also needs the lead. Each head is reported once (state in .swarm/state).
watch_prs() {
  local repos=("$@") seen="${STATE_DIR}/watch-seen" stall="${WATCH_STALL_SECS:-600}" repo n sha br mergeable age total pending
  # Default: infrafactory plus every sibling a story names (repo: <name>), so a new sibling needs no edit here.
  [[ ${#repos[@]} -gt 0 ]] || repos=(infrafactory $(sed -n 's/^repo: *//p' "${REPO_ROOT}"/docs/stories/*.md 2>/dev/null | sort -u) fakeaws fakegcp fakegenesys mockway)
  local uniq=() r
  for r in "${repos[@]}"; do [[ " ${uniq[*]-} " == *" ${r} "* ]] || uniq+=("${r}"); done
  repos=("${uniq[@]}")
  mkdir -p "${STATE_DIR}"; touch "${seen}"
  while :; do
    for repo in "${repos[@]}"; do
      while read -r n sha br mergeable age; do
        [[ -n "${n}" ]] || continue
        if [[ "${mergeable}" == CONFLICTING ]] && ! grep -qx "conflict ${sha}" "${seen}"; then
          echo "conflict ${sha}" >> "${seen}"; echo "${repo} #${n} ${br} CONFLICTS with main"; return 0
        fi
        grep -qx "done ${sha}" "${seen}" && continue
        read -r total pending < <(gh api "repos/redscaresu/${repo}/commits/${sha}/check-runs" \
          -q '"\(.check_runs|length) \([.check_runs[]|select(.status!="completed")]|length)"')
        if [[ "${total:-0}" -gt 0 && "${pending}" == 0 ]]; then
          echo "done ${sha}" >> "${seen}"; echo "${repo} #${n} ${br} checks finished on ${sha:0:7}"; return 0
        fi
        if [[ "${total:-0}" == 0 && "${age}" -gt "${stall}" ]] && ! grep -qx "stall ${sha}" "${seen}"; then
          echo "stall ${sha}" >> "${seen}"; echo "${repo} #${n} ${br} has had no checks for ${age}s"; return 0
        fi
      done < <(gh pr list -R "redscaresu/${repo}" --state open --json number,headRefName,headRefOid,mergeable,updatedAt \
        -q '.[]|select(.headRefName|startswith("story/"))|"\(.number) \(.headRefOid) \(.headRefName) \(.mergeable) \((now - (.updatedAt|fromdateiso8601))|floor)"')
    done
    if [[ "${HERDR_ENV:-}" == 1 ]]; then
      local blocked
      blocked=$(herdr agent list 2>/dev/null | json "','.join(a['name'] for a in d['result']['agents'] if a.get('agent_status')=='blocked' and a.get('name'))" 2>/dev/null || true)
      if [[ -n "${blocked}" ]] && ! grep -qx "blocked ${blocked}" "${seen}"; then
        echo "blocked ${blocked}" >> "${seen}"; echo "agent(s) blocked on a prompt: ${blocked}"; return 0
      fi
    fi
    sleep 60
  done
}

# close_tabs <label> — close every tab this script opened under <label>, and its state.
close_tabs() {
  local label="$1" state tab_label id
  for state in "${STATE_DIR}/${label}" "${STATE_DIR}/${label}"-*; do
    [[ -f "${state}" ]] || continue
    tab_label="$(basename "${state}")"
    id=$(herdr tab list --workspace "${HERDR_WORKSPACE_ID}" | python3 -c "
import json,sys
for t in json.load(sys.stdin)['result']['tabs']:
    if t.get('label') == sys.argv[1]: print(t['tab_id'])" "${tab_label}")
    [[ -n "${id}" ]] && herdr tab close "${id}" >/dev/null
    rm -f "${state}"
    echo "closed ${tab_label}"
  done
}

# Everything runs from main, called on the last line with `exit` beside it: bash reads a
# script as it executes, so a `git pull` that rewrites this file during a long `wait` would
# otherwise make it resume reading the new file mid-line.
main() {
  cmd="${1:-}"; shift || true
  case "${cmd}" in
    policy) policy "${1:?role}" ;;
    agent)  require_herdr; start_agent "$@" ;;
    story)  require_herdr; build_story "${1:?slug}" ;;
    conduct) require_herdr; start_conductor "${1:?epic}" ;;
    close)  require_herdr; close_tabs "${1:?tab}" ;;
    unblock) unblock ;;
    watch)  watch_prs "$@" ;;
    _name)  agent_name "${1:?slug}"; echo ;;                      # test hook: the agent name for a slug
    _pane)  require_herdr; next_pane "${1:?tab}" "${2:?cwd}" ;;   # layout test hook: a pane, no agent
    wait)   require_herdr; herdr agent wait "$(agent_name "${1:?name}")" --timeout "${2:-3600000}" | json "d['result']['agent']['agent_status']" ;;
    *) sed -n '2,14p' "$0"; exit 2 ;;
  esac
}

main "$@"; exit
