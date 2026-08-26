#!/usr/bin/env bash
#
# Everything `make demo-gate` needs, checked without opening or applying
# anything. Run it the morning of the talk, and again from the venue
# network.
#
# A checklist you read is a checklist you skim. This one fails.
set -uo pipefail

SCENARIO="${DEMO_SCENARIO:-block-paris}"
LABEL="layer3-gate"
fails=0

ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$*"; fails=$((fails+1)); }
warn() { printf '  \033[33m!\033[0m %s\n' "$*"; }
head_() { printf '\n\033[36m%s\033[0m\n' "$*"; }

head_ "Tooling"
command -v gh   >/dev/null && ok "gh installed"   || bad "gh missing"
command -v tofu >/dev/null && ok "tofu installed" || bad "tofu missing"
command -v go   >/dev/null && ok "go installed"   || bad "go missing"
gh auth status >/dev/null 2>&1 && ok "gh authenticated" || bad "gh not authenticated (gh auth login)"

head_ "Repository"
[ -z "$(git status --porcelain 2>/dev/null)" ] && ok "working tree clean" || bad "working tree dirty"
[ "$(git rev-parse --abbrev-ref HEAD)" = "main" ] && ok "on main" || warn "not on main"
gh label list --limit 200 2>/dev/null | grep -q "^${LABEL}" \
  && ok "'${LABEL}' label exists" || bad "'${LABEL}' label missing"
[ -f ".github/workflows/layer3-gate.yml" ] && ok "gate workflow present" || bad "gate workflow missing"

head_ "Gate inputs"
[ -d "examples/layer3-gate/${SCENARIO}" ] \
  && ok "fixture examples/layer3-gate/${SCENARIO}" || bad "fixture missing"
[ -f "examples/layer3-gate/${SCENARIO}/.terraform.lock.hcl" ] \
  && ok "trusted provider lock present" || bad "trusted lock missing — the gate refuses to run"
[ -d "docs/demo/recorded-generation/${SCENARIO}" ] \
  && ok "recorded generation present (replay path)" || bad "no recorded generation to replay"

head_ "Credentials and approvals"
if gh secret list --env layer3 2>/dev/null | grep -q SCW_SECRET_KEY; then
  ok "layer3 environment holds the SCW secrets"
else
  bad "layer3 environment is missing SCW secrets"
fi
reviewers=$(gh api repos/{owner}/{repo}/environments/layer3 2>/dev/null \
  | python3 -c 'import json,sys;d=json.load(sys.stdin);print(",".join(x["reviewer"]["login"] for r in d.get("protection_rules",[]) if r["type"]=="required_reviewers" for x in r.get("reviewers",[])))' 2>/dev/null)
[ -n "$reviewers" ] && ok "required reviewers: $reviewers" \
  || bad "no required reviewers on the layer3 environment — a label alone would apply"

head_ "The account is clean before we start"
if [ -f "$HOME/.config/infrafactory/scw-layer3.env" ]; then
  # shellcheck disable=SC1090
  set -a; . "$HOME/.config/infrafactory/scw-layer3.env"; set +a
  strays=$(python3 - <<'PY' 2>/dev/null
import json,os,urllib.request as u
tok=os.environ.get('SCW_SECRET_KEY',''); org=os.environ.get('SCW_DEFAULT_ORGANIZATION_ID','')
try:
    d=json.load(u.urlopen(u.Request(
        f'https://api.scaleway.com/account/v3/projects?organization_id={org}&page_size=100',
        headers={'X-Auth-Token':tok}), timeout=20))
    known={'default','openclaw','infrafactory'}
    print(",".join(p['name'] for p in d['projects'] if p['name'] not in known))
except Exception:
    print("UNREACHABLE")
PY
)
  case "$strays" in
    UNREACHABLE) bad "cannot reach the Scaleway API — check the venue network" ;;
    "")          ok "no stray projects in the organization" ;;
    *)           bad "stray projects from an earlier run: $strays (reap them first)" ;;
  esac
else
  warn "no ~/.config/infrafactory/scw-layer3.env — cannot check the account from here"
fi

head_ "Fallback"
if ls docs/demo/*.webm docs/demo/*.mp4 docs/demo/*.gif >/dev/null 2>&1; then
  ok "a recording exists in docs/demo/ to narrate if the network dies"
else
  bad "no recording in docs/demo/ — S148-T2 calls this non-negotiable for a live demo"
fi

printf '\n'
if [ "$fails" -eq 0 ]; then
  printf '\033[32mReady.\033[0m Run: make demo-gate\n\n'
  exit 0
fi
printf '\033[31m%d check(s) failed — fix before going on stage.\033[0m\n\n' "$fails"
exit 1
