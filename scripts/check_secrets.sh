#!/bin/sh
# Check staged files for secrets. Use in pre-commit hooks.
# Usage: bash scripts/check_secrets.sh

set -e

echo "Checking for secrets in staged files..."

# Scan config/data files for actual secret values (not code/doc references)
SECRETS_PATTERN='(SCW_ACCESS_KEY|SCW_SECRET_KEY|OPENROUTER_API_KEY|GOOGLE_APPLICATION_CREDENTIALS)=[A-Za-z0-9+/]'
if git diff --cached --diff-filter=d -U0 -- '*.yaml' '*.yml' '*.json' '*.env' '*.cfg' '*.txt' '*.tf' | grep -qE "$SECRETS_PATTERN"; then
  echo "ERROR: Potential secret value detected in staged changes!"
  git diff --cached --diff-filter=d -U0 -- '*.yaml' '*.yml' '*.json' '*.env' '*.cfg' '*.txt' '*.tf' | grep -nE "$SECRETS_PATTERN" | head -5
  echo "Remove the secret and use environment variables instead."
  exit 1
fi

# Check for private key content
if git diff --cached --diff-filter=d -U0 | grep -q 'BEGIN.*PRIVATE KEY'; then
  echo "ERROR: Private key detected in staged changes!"
  exit 1
fi

# Check for secret files
for f in .env credentials.json service-account.json token.json; do
  if git diff --cached --name-only | grep -qF "$f"; then
    echo "ERROR: Secret file '$f' is staged for commit!"
    exit 1
  fi
done

echo "No secrets detected."
