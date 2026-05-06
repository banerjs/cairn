#!/usr/bin/env bash
# Writes Terraform outputs (after apply) into a named AWS CLI profile for local use.
# Requires: terraform (initialized), aws CLI, and state containing IAM access keys.
#
#   ./scripts/export-cairn-aws-profile.sh
#   CAIRN_AWS_PROFILE=backup CAIRN_TERRAFORM_DIR=/path/to/terraform ./scripts/export-cairn-aws-profile.sh
#
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tf_dir="${CAIRN_TERRAFORM_DIR:-$root/infra/terraform}"
profile="${CAIRN_AWS_PROFILE:-cairn}"

for cmd in terraform aws; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: '$cmd' not found in PATH" >&2
    exit 1
  fi
done

if [[ ! -d "$tf_dir/.terraform" ]]; then
  echo "error: run terraform init in $tf_dir first" >&2
  exit 1
fi

cd "$tf_dir"

access_key_id="$(terraform output -raw access_key_id)"
secret_access_key="$(terraform output -raw secret_access_key)"
region="$(terraform output -raw aws_region)"

aws configure set aws_access_key_id "$access_key_id" --profile "$profile"
aws configure set aws_secret_access_key "$secret_access_key" --profile "$profile"
aws configure set region "$region" --profile "$profile"

echo "Configured AWS CLI profile '$profile' (region $region)."
echo "Example: export AWS_PROFILE=$profile"
