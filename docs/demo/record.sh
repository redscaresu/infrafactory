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
echo "InfraFactory: scenario-driven IaC with LLM agents + mock-backed validation"
sleep 2
echo ""
echo "Step 1 - Pick a scenario:"
sleep 1
head -25 scenarios/training/web-app-paris.yaml
sleep 5
echo ""
echo "Step 2 - Run it against the Scaleway mock (mockway on :8080):"
sleep 2
infrafactory run scenarios/training/web-app-paris.yaml
echo ""
sleep 3
echo "Step 3 - See the generated HCL:"
sleep 1
ls -la output/web-app-paris/ | head -10
sleep 4
echo ""
echo "Convergence: structured failures -> next prompt -> retry. No human in the loop."
sleep 3
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
