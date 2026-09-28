#!/usr/bin/env bash
# Regenerates the deriveTopologyAWS fixtures: applies main.tf once per
# variant against a running fakeaws (>= 5cf7693) and writes the trimmed
# /mock/state as <variant>.json beside this script.
#
#   (cd ../fakeaws && go build -o fakeaws ./cmd/fakeaws && ./fakeaws --port 8082 --db :memory: &)
#   FAKEAWS_URL=http://127.0.0.1:8082 internal/harness/testdata/topology/aws/capture.sh
#
# Credentials are fakes, every endpoint is fakeaws, and apply runs behind
# a dead proxy, so nothing can reach real AWS.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
url="${FAKEAWS_URL:-http://127.0.0.1:8082}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cp "$here/main.tf" "$work/"

for v in $(env | sed -n 's/^\(AWS_[A-Z0-9_]*\)=.*/\1/p'); do unset "$v"; done
export AWS_ACCESS_KEY_ID=fake AWS_SECRET_ACCESS_KEY=fake AWS_REGION=us-east-1
export AWS_EC2_METADATA_DISABLED=true AWS_CONFIG_FILE="$work/none" AWS_SHARED_CREDENTIALS_FILE="$work/none"
export AWS_ENDPOINT_URL_EC2="$url/ec2/region/us-east-1" AWS_ENDPOINT_URL_STS="$url/sts"

rule() { # from to protocol cidr_blocks ipv6_cidr_blocks
  echo "ingress=[{from_port=$1,to_port=$2,protocol=\"$3\",cidr_blocks=$4,ipv6_cidr_blocks=$5}]"
}

variants=(
  "positive"
  "unattached_group|attach_open=false"
  "no_igw_route|igw_route=false"
  "no_association|associate=false"
  "no_public_ip|public_ip=false"
  "private_source|$(rule 80 80 tcp '["10.0.0.0/16"]' '[]')"
  "ipv6_only|$(rule 80 80 tcp '[]' '["::/0"]')"
  "port_range|$(rule 80 90 tcp '["0.0.0.0/0"]' '[]')"
  "all_protocol|$(rule 0 0 -1 '["0.0.0.0/0"]' '[]')"
  "no_public_rule|ingress=[]"
)

tofu -chdir="$work" init -input=false >/dev/null
for entry in "${variants[@]}"; do
  name="${entry%%|*}"
  args=()
  [[ "$entry" == *"|"* ]] && args=(-var "${entry#*|}")
  curl -sf -X POST "$url/mock/reset" >/dev/null
  rm -f "$work/terraform.tfstate"
  HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 NO_PROXY=127.0.0.1,localhost \
    tofu -chdir="$work" apply -input=false -auto-approve ${args[@]+"${args[@]}"} >/dev/null
  curl -sf "$url/mock/state" | jq '{schema_version, iam: {}, s3: {}, ec2: (.ec2 | {
    instances, security_groups, subnets, internet_gateways,
    route_tables, routes, route_table_associations})}' >"$here/$name.json"
  echo "captured $name"
done
