#!/usr/bin/env bash
# Regenerates the no_instance_profile plan fixtures: plans each variant
# directory, plus the e2e web-step-one HCL, against a running fakeaws and
# writes `tofu show -json` of the plan as <variant>.json beside this script.
#
#   (cd ../fakeaws && go build -o fakeaws ./cmd/fakeaws && ./fakeaws --port 8082 --db :memory: &)
#   FAKEAWS_URL=http://127.0.0.1:8082 internal/harness/testdata/instance-profile/aws/capture.sh
#
# Credentials are fakes, every endpoint is fakeaws, and plan runs behind
# a dead proxy, so nothing can reach real AWS.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../../../.." && pwd)"
url="${FAKEAWS_URL:-http://127.0.0.1:8082}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

for v in $(env | sed -n 's/^\(AWS_[A-Z0-9_]*\)=.*/\1/p'); do unset "$v"; done
export AWS_ACCESS_KEY_ID=fake AWS_SECRET_ACCESS_KEY=fake AWS_REGION=us-east-1
export AWS_EC2_METADATA_DISABLED=true AWS_CONFIG_FILE="$work/none" AWS_SHARED_CREDENTIALS_FILE="$work/none"
export AWS_ENDPOINT_URL_EC2="$url/ec2/region/us-east-1" AWS_ENDPOINT_URL_STS="$url/sts" AWS_ENDPOINT_URL_IAM="$url/iam"

capture() { # name, then the files that make up its configuration
  local name="$1" dir="$work/$1"
  shift
  mkdir -p "$dir"
  cp -R "$@" "$dir/"
  cp "$here/providers.tf" "$dir/"
  tofu -chdir="$dir" init -input=false >/dev/null
  HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 NO_PROXY=127.0.0.1,localhost \
    tofu -chdir="$dir" plan -input=false -out=plan.out >/dev/null
  tofu -chdir="$dir" show -json plan.out | jq 'del(.timestamp)' >"$here/$name.json"
  echo "captured $name"
}

for variant in literal_name unknown_name child_module launch_template launch_template_dynamic launch_template_unknown_count launch_template_no_profile spot_instance launch_configuration; do
  capture "$variant" "$here/$variant/."
done
touch "$work/infrafactory-user-data.sh"
capture web_step_one "$repo/internal/e2e/testdata/aws-web-step-one/web-step-one.tf" "$work/infrafactory-user-data.sh"
