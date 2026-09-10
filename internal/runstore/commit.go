package runstore

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// commitTimeout bounds the two git calls. Both read local refs; a
// second is already generous, and a hung git must not delay a run.
const commitTimeout = time.Second

// CurrentCommit returns the working tree's git revision, suffixed
// "-dirty" when it has uncommitted changes, or "" when it cannot be
// determined.
//
// Best-effort on purpose. This is provenance for a run record, not a
// gate: running outside a checkout, without git installed, or in a
// repository with no commits yet are all legitimate, and none of them
// is a reason to fail a run. What is NOT acceptable is a confident
// wrong value, so every failure path yields "".
//
// The dirty flag matters as much as the hash. A run whose policies and
// pitfalls came from uncommitted edits cannot be reproduced from the
// hash alone, and saying so is the difference between provenance and
// decoration.
func CurrentCommit(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), commitTimeout)
	defer cancel()

	rev := gitOutput(ctx, dir, "rev-parse", "HEAD")
	if rev == "" {
		return ""
	}
	// --porcelain is empty exactly when the tree is clean. An ERROR here
	// must not be read as clean: it would stamp a plain hash on a run
	// that may have been generated from edits, which is the specific
	// falsehood this field exists to prevent.
	status, err := gitStatus(ctx, dir)
	if err != nil {
		return rev + "-dirty-unknown"
	}
	if status != "" {
		return rev + "-dirty"
	}
	return rev
}

func gitOutput(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitStatus(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
