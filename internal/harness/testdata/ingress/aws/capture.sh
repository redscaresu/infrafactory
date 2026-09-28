#!/usr/bin/env bash
# Regenerates the default_deny_ingress fixtures from hashicorp/aws
# 5.100.0 against a running fakeaws at the CI pin (FAKEAWS_SHA in
# .github/workflows/ci.yml):
#   - plan.json, network.json, network_open.json: trimmed `tofu show
#     -json` plans of plan/ and of network/ (whose 0.0.0.0/0 variant is
#     network.tf with the CIDR swapped and the group renamed open);
#   - state_<verdict>_<variant>.json: trimmed /mock/state after applying
#     state/ once per variant.
#
#   (cd ../fakeaws && go build -o fakeaws ./cmd/fakeaws && ./fakeaws --port 8082 --db :memory: &)
#   FAKEAWS_URL=http://127.0.0.1:8082 internal/harness/testdata/ingress/aws/capture.sh
#
# Credentials are fakes, every endpoint is fakeaws, and plan and apply
# run behind a dead proxy, so nothing can reach real AWS.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
url="${FAKEAWS_URL:-http://127.0.0.1:8082}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

for v in $(env | sed -n 's/^\(AWS_[A-Z0-9_]*\)=.*/\1/p'); do unset "$v"; done
export AWS_ACCESS_KEY_ID=fake AWS_SECRET_ACCESS_KEY=fake AWS_REGION=us-east-1
export AWS_EC2_METADATA_DISABLED=true AWS_CONFIG_FILE="$work/none" AWS_SHARED_CREDENTIALS_FILE="$work/none"
export AWS_ENDPOINT_URL_EC2="$url/ec2/region/us-east-1" AWS_ENDPOINT_URL_STS="$url/sts"
sealed() { HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 NO_PROXY=127.0.0.1,localhost "$@"; }

# Keeps what the policy reads: the security-group resource changes, and
# enough configuration to find each one's `ingress` expression.
trim_plan='
def mod: {resources: [.resources[]? | select(.type | test("security_group"))
    | {address} + (if .expressions.ingress then {expressions: {ingress: .expressions.ingress}} else {} end)]}
  + (if .module_calls then {module_calls: (.module_calls | map_values({module: (.module | mod)}))} else {} end);
{resource_changes: [.resource_changes[] | select(.type | test("security_group"))
    | {address, mode, type, name, change: (.change | {actions, after, after_unknown})}
      + (if .module_address then {module_address} else {} end)],
 configuration: {root_module: (.configuration.root_module | mod)}}'

plan() { # dir output
  tofu -chdir="$1" init -input=false >/dev/null
  sealed tofu -chdir="$1" plan -input=false -out=plan.bin >/dev/null
  tofu -chdir="$1" show -json plan.bin | jq "$trim_plan" >"$here/$2"
  echo "captured $2"
}

cp -R "$here/plan" "$work/plan"
plan "$work/plan" plan.json
cp -R "$here/network" "$work/network"
plan "$work/network" network.json
sed -i.bak -e 's#"10.0.0.0/16"#"0.0.0.0/0"#' -e 's#"aws_security_group" "app"#"aws_security_group" "open"#' "$work/network/network.tf"
plan "$work/network" network_open.json

rule() { # from to protocol cidr_blocks ipv6_cidr_blocks prefix_list_ids from_peer
  echo "ingress=[{from_port=$1,to_port=$2,protocol=\"$3\",cidr_blocks=$4,ipv6_cidr_blocks=$5,prefix_list_ids=$6,from_peer=$7}]"
}

variants=(
  "deny_ssh_ipv4|$(rule 22 22 tcp '["0.0.0.0/0"]' '[]' '[]' false)"
  "deny_ssh_ipv6|$(rule 22 22 tcp '[]' '["::/0"]' '[]' false)"
  "deny_all|$(rule 0 0 -1 '["0.0.0.0/0"]' '[]' '[]' false)"
  "deny_prefix_list|$(rule 443 443 tcp '[]' '[]' '["pl-12345678"]' false)"
  "pass_ssh_private|$(rule 22 22 tcp '["10.0.0.0/16"]' '[]' '[]' false)"
  "pass_web|$(rule 80 80 tcp '["0.0.0.0/0"]' '[]' '[]' false)"
  "pass_all_from_peer|$(rule 0 0 -1 '[]' '[]' '[]' true)"
  "pass_no_ingress|ingress=[]"
)

cp -R "$here/state" "$work/state"
tofu -chdir="$work/state" init -input=false >/dev/null
for entry in "${variants[@]}"; do
  name="${entry%%|*}"
  curl -sf -X POST "$url/mock/reset" >/dev/null
  rm -f "$work/state/terraform.tfstate"
  sealed tofu -chdir="$work/state" apply -input=false -auto-approve -var "${entry#*|}" >/dev/null
  curl -sf "$url/mock/state" | jq '{ec2: {security_groups: .ec2.security_groups}}' >"$here/state_$name.json"
  echo "captured state_$name.json"
done

# The one state no apply produces: a group whose ip_permissions key is
# absent, which must deny rather than read as a group with no rules.
jq '.ec2.security_groups |= map(if .group_name == "web" then del(.ip_permissions) else . end)' \
  "$here/state_pass_ssh_private.json" >"$here/state_deny_no_ip_permissions.json"
echo "captured state_deny_no_ip_permissions.json"
