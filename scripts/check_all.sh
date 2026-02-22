#!/usr/bin/env bash
set -euo pipefail

echo "[1/2] go test ./..."
go test ./...

echo "[2/2] doc hygiene (--staged)"
bash scripts/check_doc_hygiene.sh --staged

echo "All checks passed."
