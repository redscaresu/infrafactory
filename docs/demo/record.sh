#!/usr/bin/env bash
# Records a deterministic ~60-second demo of an infrafactory run end-to-end.
# Output: docs/demo/infrafactory.cast (asciinema) which can be uploaded to
# asciinema.org OR rendered to GIF via `agg`/`svg-term-cli`.
#
# Prerequisites:
#   - asciinema installed (brew install asciinema OR apt-get install asciinema)
#   - mocks already running:  make mocks-up
#   - OPENROUTER_API_KEY env var set (or claude credentials wired)
#
# Usage:
#   ./docs/demo/record.sh
#
# Then either:
#   asciinema upload docs/demo/infrafactory.cast
#   # OR render to GIF:
#   agg docs/demo/infrafactory.cast docs/demo/infrafactory.gif

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${REPO_ROOT}"

OUTPUT="docs/demo/infrafactory.cast"

if ! command -v asciinema &>/dev/null; then
  echo "ERROR: asciinema not installed. brew install asciinema (or apt-get install asciinema)." >&2
  exit 1
fi

if ! curl -fsS http://127.0.0.1:8080/mock/state >/dev/null 2>&1; then
  echo "ERROR: mockway not running on :8080. Run 'make mocks-up' first." >&2
  exit 1
fi

# Write the demo script to a temp file. We invoke asciinema with `-c`
# so it spawns a subshell — passing a multi-line heredoc through
# `bash -c "$SCRIPT"` triple-escapes badly (asciinema runs the -c
# argument through `/bin/sh`, which re-parses our inner quoting and
# breaks on syntax like `1) Pick a scenario:`). A file path is
# unambiguous.
mkdir -p docs/demo
SCRIPT_FILE="$(mktemp -t infrafactory-demo.XXXXXX.sh)"
trap 'rm -f "${SCRIPT_FILE}"' EXIT

cat > "${SCRIPT_FILE}" <<'EOS'
#!/usr/bin/env bash
set -e
PS1='$ '
clear
echo "# InfraFactory: scenario-driven IaC with LLM agents + mock-backed validation"
echo ""
sleep 2

# --- The CLI surface ---
echo "$ infrafactory --help"
sleep 1
infrafactory --help 2>&1 | head -16
sleep 5

# --- Setup: mocks must be running (pre-staged) ---
echo ""
echo "$ infrafactory mock status"
sleep 1
infrafactory mock status 2>&1 | sed -n '/^Command:/,/^{/p' | head -10
sleep 3

# --- Step 1: scenario YAML = the intent ---
echo ""
echo "# Step 1 — Declare intent (scenarios/training/registry-paris.yaml):"
sleep 1
cat scenarios/training/registry-paris.yaml
sleep 5

# --- Step 2: the magic — 3-phase LLM + 4-layer validation ---
echo ""
echo "# Step 2 — Run the pipeline (Claude generates → mockway validates → retries on failure):"
sleep 2
infrafactory run scenarios/training/registry-paris.yaml
sleep 2

# --- Step 3: the actual HCL the LLM converged on ---
echo ""
echo "# Step 3 — The generated OpenTofu the model converged on:"
sleep 1
echo ""
echo "$ ls output/registry-paris/"
ls output/registry-paris/ | grep -E '\.tf$|\.tfstate$' | head -6
sleep 3

echo ""
echo "$ cat output/registry-paris/main.tf"
sleep 1
cat output/registry-paris/main.tf
sleep 4

echo ""
echo "$ cat output/registry-paris/providers.tf"
sleep 1
cat output/registry-paris/providers.tf
sleep 4

echo ""
echo "$ cat output/registry-paris/variables.tf"
sleep 1
cat output/registry-paris/variables.tf
sleep 5

# --- Wrap ---
echo ""
echo "# That's the loop:"
echo "#   scenario YAML  ->  LLM generates HCL  ->  mock validates  ->  retry on failure"
echo "# Subsecond mock feedback. No cloud credentials. No 90-second apply-cycle waits."
sleep 4
EOS
chmod +x "${SCRIPT_FILE}"

echo "Recording to ${OUTPUT}..."
echo "Pre-warmed dependencies. The recording starts in 3s."
sleep 3
asciinema rec --overwrite -c "bash ${SCRIPT_FILE}" "${OUTPUT}"
echo "Done: ${OUTPUT}"
echo ""
echo "Upload:  asciinema upload ${OUTPUT}"
echo "Render:  agg ${OUTPUT} docs/demo/infrafactory.gif"
