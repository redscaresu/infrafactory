#!/usr/bin/env bash
set -euo pipefail

echo "[1/3] go test ./..."
go test ./...

echo "[2/3] doc hygiene (--staged)"
bash scripts/check_doc_hygiene.sh --staged

echo "[3/3] benchmark guard (env-gated)"
bash scripts/check_benchmarks.sh

echo "All checks passed."
