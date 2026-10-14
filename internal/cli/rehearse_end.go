package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/spf13/cobra"
)

const rehearseEndSchema = "infrafactory.rehearse.end.v1"

// rehearseEnd is what `rehearse end` runs with. A zero field takes its
// default.
type rehearseEnd struct {
	// wd is the current directory: os.Getwd by default.
	wd string
	// notify catches a signal as a cancel: signal.NotifyContext by default.
	notify func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
}

// newRehearseEndCmd is `rehearse end <id>` (ADR-0042 § The walls family:
// `rehearse end`). It reads no infrafactory.yaml and takes no flags.
func newRehearseEndCmd(e rehearseEnd) *cobra.Command {
	if e.notify == nil {
		e.notify = signal.NotifyContext
	}
	return &cobra.Command{
		Use:   "end <id>",
		Short: "Stop a rehearsal's fakeaws once it proves itself, then delete the rehearsal",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return rehearseUsage(cmd, err)
			}
			return nil
		},
		RunE: e.run,
	}
}

func (e rehearseEnd) run(cmd *cobra.Command, args []string) error {
	mode, err := outputModeFromCommand(cmd)
	if err != nil {
		return err
	}
	if e.wd == "" {
		if e.wd, err = os.Getwd(); err != nil {
			return &CLIError{Op: cmd.CommandPath(), Code: errorCodeCommandFailed, Err: err}
		}
	}

	// A signal is a cancel through a guard, so it never ends the process
	// mid-stop: before the stop, end refuses; after, it finishes.
	ctx, stop := notifyGuarded(cmd.Context(), e.notify)
	result, err := e.end(ctx, args[0])
	stop()
	result.Schema, result.Status = rehearseEndSchema, CommandStatusSuccess
	if err != nil {
		result.Status, result.Refusal = CommandStatusFailed, err.Error()
	}
	if writeErr := result.write(cmd, mode); writeErr != nil {
		return writeErr
	}
	switch {
	case errors.Is(err, errRehearsalBadID):
		return rehearseUsage(cmd, err)
	case err != nil:
		return &CLIError{Op: cmd.CommandPath(), Code: errorCodeCommandFailed, Err: err}
	}
	return nil
}

// end finds rehearsal id with rehearsalOpen, stops its fakeaws when it
// proves itself (rehearsalEndProof), then deletes the rehearsal whole. A
// missing rehearsal is ended already. A refused record signals nothing
// and deletes, and a live dead proxy is a warning: end runs no tofu. A
// cancel is refused up to the stop, never after it.
func (e rehearseEnd) end(ctx context.Context, id string) (rehearseEndResult, error) {
	result := rehearseEndResult{ID: id, Deleted: []string{}}
	repo, err := rehearsalRepo(e.wd)
	if err != nil {
		return result, err
	}
	opened, err := rehearsalOpen(repo, id)
	var why string
	switch {
	case errors.Is(err, errRehearsalMissing):
		result.Fakeaws = "not signalled: " + err.Error()
		return result, nil
	case errors.Is(err, errRehearsalRecord):
		why = err.Error()
	case errors.Is(err, errRehearsalProxyLive):
		result.Warnings = append(result.Warnings, "end runs no tofu, so this does not stop it: "+err.Error())
		why = rehearsalEndProof(ctx, opened.record)
	case err != nil:
		return result, err
	default:
		why = rehearsalEndProof(ctx, opened.record)
	}
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("end is cancelled before the stop, with nothing signalled or deleted: %w", err)
	}

	if why != "" {
		result.Fakeaws = "not signalled: " + why
	} else if err := rehearsalStopRecorded(opened.record); err != nil {
		result.Fakeaws = err.Error()
		return result, fmt.Errorf("%w; nothing is deleted, so a later end can retry", err)
	} else {
		result.Fakeaws = "stopped"
	}
	result.Deleted, err = rehearsalDelete(opened.dir)
	return result, err
}

// rehearsalEndProof is why end must not signal the recorded fakeaws, or
// "" when it proves itself: the record names a pid other than 0 and 1
// (kill(-1) reaches every process the user owns), a port and a token,
// /healthz on the port answers 200, and GET /mock/requests answers 200
// with the token. Any other answer or error signals nothing.
func rehearsalEndProof(ctx context.Context, r rehearsalRecord) string {
	if r.PID <= 1 || r.Port <= 0 || r.Port > 65535 || r.Token == "" {
		return fmt.Sprintf("the record names no fakeaws to signal: pid %d port %d, token set: %t", r.PID, r.Port, r.Token != "")
	}
	ctx, cancel := context.WithTimeout(ctx, rehearsalDialTimeout)
	defer cancel()
	client, base := rehearsalFakeawsClient(), rehearsalFakeawsURL(r.Port)
	for _, check := range []struct{ path, token, name string }{
		{"/healthz", "", "/healthz"},
		{"/mock/requests", r.Token, "GET /mock/requests with the record's token"},
	} {
		code, err := rehearsalFakeawsStatus(ctx, client, base+check.path, check.token)
		switch {
		case err != nil:
			return fmt.Sprintf("pid %d port %d: %s failed: %v", r.PID, r.Port, check.name, err)
		case code != http.StatusOK:
			return fmt.Sprintf("pid %d port %d: %s answered %d", r.PID, r.Port, check.name, code)
		}
	}
	return ""
}

// rehearsalStopRecorded stops the recorded fakeaws's process group, done
// only once the group is gone and its port refuses a dial: a port still
// answering after the SIGKILL fallback is `stop failed: pid P port N`.
func rehearsalStopRecorded(r rehearsalRecord) error {
	f := rehearsalFakeaws{PID: r.PID, Port: r.Port, gone: new(atomic.Bool)}
	return f.stopUntil(func() bool {
		return f.groupGone() && rehearsalProxyRefuses(context.Background(), "", rehearsalFakeawsURL(f.Port)) == nil
	})
}

// rehearsalDelete lists every path under dir, relative, then removes dir
// whole, and returns the list. If the removal fails part way, it returns
// what is gone.
func rehearsalDelete(dir string) ([]string, error) {
	paths := []string{}
	err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir {
			rel, _ := filepath.Rel(dir, path)
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return []string{}, fmt.Errorf("the rehearsal %s cannot be listed, so nothing is deleted: %w", dir, err)
	}
	if err := os.RemoveAll(dir); err != nil {
		gone := []string{}
		for _, rel := range paths {
			if _, statErr := os.Lstat(filepath.Join(dir, rel)); errors.Is(statErr, fs.ErrNotExist) {
				gone = append(gone, rel)
			}
		}
		return gone, fmt.Errorf("the rehearsal %s cannot be removed whole: %w", dir, err)
	}
	return paths, nil
}

// rehearseEndResult is what end reports: the id, what became of its
// fakeaws (stopped, not signalled: <why> or stop failed: <why>), any
// warnings, and every path deleted, relative to <id>. Never the token.
type rehearseEndResult struct {
	Schema   string        `json:"schema"`
	Status   CommandStatus `json:"status"`
	ID       string        `json:"id"`
	Fakeaws  string        `json:"fakeaws,omitempty"`
	Warnings []string      `json:"warnings,omitempty"`
	Deleted  []string      `json:"deleted"`
	Refusal  string        `json:"refusal,omitempty"`
}

func (r rehearseEndResult) write(cmd *cobra.Command, mode OutputMode) error {
	out := cmd.OutOrStdout()
	if mode == OutputModeJSON {
		encoded, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return fmt.Errorf("encode end result: %w", err)
		}
		_, err = fmt.Fprintf(out, "%s\n", encoded)
		return err
	}
	var human strings.Builder
	fmt.Fprintf(&human, "Command: rehearse end\nStatus: %s\nRehearsal: %s\n", r.Status, r.ID)
	if r.Fakeaws != "" {
		fmt.Fprintf(&human, "Fakeaws: %s\n", r.Fakeaws)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(&human, "Warning: %s\n", warning)
	}
	if r.Refusal != "" {
		fmt.Fprintf(&human, "Refused: %s\n", r.Refusal)
	}
	if len(r.Deleted) == 0 {
		human.WriteString("Deleted: nothing\n")
	}
	for _, path := range r.Deleted {
		fmt.Fprintf(&human, "Deleted: %s\n", path)
	}
	_, err := io.WriteString(out, human.String())
	return err
}
