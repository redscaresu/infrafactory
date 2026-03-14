#!/usr/bin/env bash
set -euo pipefail

if [ -d "cmd/infrafactory/ui/build" ]; then
	echo "[1/3] go test ./..."
	go test ./...
else
	echo "[1/3] go test -tags noui ./... (embedded UI assets missing)"
	go test -tags noui ./...
fi

echo "[2/3] doc hygiene (--staged)"
bash scripts/check_doc_hygiene.sh --staged

echo "[3/3] benchmark guard (env-gated)"
bash scripts/check_benchmarks.sh

echo "All checks passed."
