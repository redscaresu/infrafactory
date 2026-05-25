#!/usr/bin/env bash
# Records the CLI demo for the README.
#
# Output: docs/demo/infrafactory.cast (raw recording) +
#         docs/demo/infrafactory.gif (rendered for README embed).
#
# The .cast is produced by `asciinema rec` (terminal recorder) and
# the .gif is rendered from it by `agg`. Only the .gif is embedded
# in the README; the .cast is kept as the regeneration source.
#
# Prerequisites:
#   - asciinema + agg on PATH:  brew install asciinema agg
#   - mocks already running:    make mocks-up
#   - LLM credential in env:    Claude CLI on PATH OR OPENROUTER_API_KEY exported
#
# Usage:
#   ./docs/demo/record.sh

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
# Dwell times sized for first-time viewers — same reading-time rules
# as the UI demos: dense code blocks 8–12s; short lists 3–4s;
# single headings 3–4s; result summaries 5–7s; closing pause 4s.
set -e
PS1='$ '
clear
echo "# InfraFactory: scenario-driven IaC with LLM agents + mock-backed validation"
echo ""
sleep 3

# --- The CLI surface (16 lines of help — dense block) ---
echo "$ infrafactory --help"
sleep 1
infrafactory --help 2>&1 | head -16
sleep 8

# --- Setup: mocks must be running (pre-staged) ---
echo ""
echo "$ infrafactory mock status"
sleep 1
infrafactory mock status 2>&1 | sed -n '/^Command:/,/^{/p' | head -10
sleep 4

# --- Step 1: scenario YAML = the intent (23 lines — dense block) ---
echo ""
echo "# Step 1 — Declare intent (scenarios/training/gcp-pubsub.yaml):"
sleep 1
cat scenarios/training/gcp-pubsub.yaml
sleep 11

# --- Step 2: the magic — 3-phase LLM + 4-layer validation ---
echo ""
echo "# Step 2 — Run the pipeline (Claude generates → fakegcp validates → retries on failure):"
sleep 2
infrafactory run scenarios/training/gcp-pubsub.yaml
# Viewer needs time to read the "Status: success" + stages summary
# block that lands at the end of the run output. Bumped from 2s.
sleep 6

# --- Step 3: the actual HCL the LLM converged on ---
echo ""
echo "# Step 3 — The generated OpenTofu the model converged on:"
sleep 1
echo ""
echo "$ ls output/gcp-pubsub/"
ls output/gcp-pubsub/ | grep -E '\.tf$|\.tfstate$' | head -6
sleep 3

echo ""
echo "$ cat output/gcp-pubsub/main.tf"
sleep 1
cat output/gcp-pubsub/main.tf
sleep 5

echo ""
echo "$ cat output/gcp-pubsub/providers.tf"
sleep 1
cat output/gcp-pubsub/providers.tf
sleep 6

echo ""
echo "$ cat output/gcp-pubsub/variables.tf"
sleep 1
cat output/gcp-pubsub/variables.tf
sleep 7

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
echo "Recorded: ${OUTPUT}"

GIF_OUTPUT="docs/demo/infrafactory.gif"
if command -v agg &>/dev/null; then
  echo "Rendering GIF -> ${GIF_OUTPUT}..."
  agg --theme github-dark --font-size 14 --speed 1.4 "${OUTPUT}" "${GIF_OUTPUT}"
  echo "Rendered: ${GIF_OUTPUT}"
else
  echo "WARN: agg not installed; skipping GIF render. brew install agg, then:"
  echo "      agg --theme github-dark --font-size 14 --speed 1.4 ${OUTPUT} ${GIF_OUTPUT}"
fi
