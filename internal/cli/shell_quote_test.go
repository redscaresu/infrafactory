package cli

import (
	"os/exec"
	"strings"
	"testing"
)

// The recovery command is read at the worst possible moment -- a failed
// run that may have left real resources billing. If the operator pastes
// it and the shell mangles the path, the cleanup does not happen.
func TestShellQuoteRoundTripsThroughARealShell(t *testing.T) {
	paths := []string{
		"scenarios/training/block-paris.yaml",
		"/Users/some one/go/src/infrafactory/scenarios/training/block-paris.yaml",
		"/tmp/dir with spaces/sc.yaml",
		"/tmp/it's-quoted/sc.yaml",
		"/tmp/semi;colon/sc.yaml",
		"/tmp/dollar$var/sc.yaml",
		"/tmp/back`tick`/sc.yaml",
		"/tmp/star*glob/sc.yaml",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			// printf %s through /bin/sh is the same parse the operator's
			// shell would apply to the pasted command.
			out, err := exec.Command("/bin/sh", "-c", "printf %s "+shellQuote(path)).Output()
			if err != nil {
				t.Fatalf("shell rejected the quoted path %q: %v", shellQuote(path), err)
			}
			if got := string(out); got != path {
				t.Errorf("shell saw %q, want %q (quoted as %s)", got, path, shellQuote(path))
			}
		})
	}
}

// Ordinary paths must stay unquoted -- this line is read under stress and
// gratuitous quoting makes it harder to scan.
func TestShellQuoteLeavesOrdinaryPathsAlone(t *testing.T) {
	path := "scenarios/training/block-paris.yaml"
	if got := shellQuote(path); got != path {
		t.Errorf("got %q, want it unquoted as %q", got, path)
	}
}

func TestShellQuoteQuotesEmptyString(t *testing.T) {
	if got := shellQuote(""); got != "''" {
		t.Errorf("got %q, want %q", got, "''")
	}
}

// The end-to-end claim: a failed Layer 3 run whose scenario path contains
// a space still prints a command that runs.
func TestRecoveryCommandIsPasteableForPathWithSpace(t *testing.T) {
	failures := []FailureSummary{{Layer: "sandbox_deploy", Stage: "orphan_sweep", Detail: "unreachable"}}

	annotateWithRecoveryCommand(failures, "/tmp/my scenarios/block-paris.yaml")

	if !strings.Contains(failures[0].Detail, `'/tmp/my scenarios/block-paris.yaml'`) {
		t.Errorf("recovery command did not quote a path with a space: %q", failures[0].Detail)
	}
}
