package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
)

const rehearsePrepareSchema = "infrafactory.rehearse.prepare.v1"

// rehearsePrepare is what `rehearse prepare` runs with. A zero field takes
// its default.
type rehearsePrepare struct {
	// runner runs tofu: execCommandRunner{} by default.
	runner harness.CommandRunner
	// wd is the current directory: os.Getwd by default.
	wd string
	// newID names a new rehearsal: a UTC timestamp and a random suffix by
	// default.
	newID func() string
	// notify catches a signal as a cancel: signal.NotifyContext by default.
	notify func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
	// fakeawsBin resolves the fakeaws binary from fakeawsBinPath:
	// rehearsalFakeawsBinary by default.
	fakeawsBin func(ctx context.Context, runner harness.CommandRunner, flagPath string) (string, error)
	// fakeawsBinPath is the binary fakeawsBin is passed, "" for the pinned
	// install. --fakeaws-bin, when given, replaces it.
	fakeawsBinPath string
	// startFakeaws starts the rehearsal's fakeaws and returns its stop:
	// rehearsalStartFakeaws by default.
	startFakeaws func(context.Context, rehearsalFakeawsOpts) (rehearsalFakeaws, func() error, error)
}

// rehearsalStartFakeawsWithStop is rehearsalStartFakeaws with its stop, as
// startFakeaws returns it.
func rehearsalStartFakeawsWithStop(ctx context.Context, opts rehearsalFakeawsOpts) (rehearsalFakeaws, func() error, error) {
	f, err := rehearsalStartFakeaws(ctx, opts)
	return f, f.stop, err
}

// newRehearsePrepareCmd is `rehearse prepare <dir>` (ADR-0042). It reads no
// infrafactory.yaml and sets no persistent pre-run: the group's makes a
// missing required flag a usage error.
func newRehearsePrepareCmd(p rehearsePrepare) *cobra.Command {
	if p.runner == nil {
		p.runner = execCommandRunner{}
	}
	if p.newID == nil {
		p.newID = func() string { return time.Now().UTC().Format("20060102T150405Z") + "-" + randomIDSuffix() }
	}
	if p.notify == nil {
		p.notify = signal.NotifyContext
	}
	if p.fakeawsBin == nil {
		p.fakeawsBin = rehearsalFakeawsBinary
	}
	if p.startFakeaws == nil {
		p.startFakeaws = rehearsalStartFakeawsWithStop
	}
	cmd := &cobra.Command{
		Use:   "prepare <dir>",
		Short: "Copy <dir> and its local modules into a new rehearsal and gate the copy before any tofu",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return rehearseUsage(cmd, err)
			}
			return nil
		},
		RunE: p.run,
	}
	cmd.Flags().String("account", "", "the AWS account id the rehearsal copies (12 digits)")
	cmd.Flags().String("region", "", "the AWS region the code must target")
	cmd.Flags().String("var-file", "", "a .tfvars file, copied into the rehearsal; it overrides the code's terraform.tfvars and *.auto.tfvars")
	cmd.Flags().String("fakeaws-bin", "", "an absolute path to a fakeaws binary built at infrafactory's pin; unset, prepare installs the pin into the user cache")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("region")
	return cmd
}

func (p rehearsePrepare) run(cmd *cobra.Command, args []string) error {
	account, _ := cmd.Flags().GetString("account")
	region, _ := cmd.Flags().GetString("region")
	varFile, _ := cmd.Flags().GetString("var-file")
	if !harness.ValidAWSAccountID(account) {
		return rehearseUsage(cmd, fmt.Errorf("--account %q is not a 12-digit AWS account id", account))
	}
	if !harness.ValidAWSRegion(region) {
		return rehearseUsage(cmd, fmt.Errorf("--region %q is not an AWS region name", region))
	}
	// A --fakeaws-bin that is not absolute is refused before any work; an
	// empty one is never read as unset, which would install the pin where
	// a binary was asked for.
	if cmd.Flags().Changed("fakeaws-bin") {
		if p.fakeawsBinPath, _ = cmd.Flags().GetString("fakeaws-bin"); !filepath.IsAbs(p.fakeawsBinPath) {
			return rehearseUsage(cmd, fmt.Errorf("--fakeaws-bin %q is not an absolute path; leave the flag out for the pinned install", p.fakeawsBinPath))
		}
	}
	mode, err := outputModeFromCommand(cmd)
	if err != nil {
		return err
	}
	if p.wd == "" {
		if p.wd, err = os.Getwd(); err != nil {
			return &CLIError{Op: cmd.CommandPath(), Code: errorCodeCommandFailed, Err: err}
		}
	}

	// A signal cancels prepare through a guard, so the runner does not
	// raise it again once tofu stops: prepare removes the half-made
	// rehearsal and reports the refusal, exit 1. A signal outside tofu is
	// refused at prepare's next check (gateAndInit).
	ctx, stop := notifyGuarded(cmd.Context(), p.notify)
	result, err := p.prepare(ctx, args[0], account, region, varFile)
	stop()
	result.Schema, result.Status = rehearsePrepareSchema, CommandStatusSuccess
	if err != nil {
		result = rehearsePrepareResult{Schema: rehearsePrepareSchema, Status: CommandStatusFailed, Refusal: err.Error()}
	}
	if writeErr := result.write(cmd, mode); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return &CLIError{Op: cmd.CommandPath(), Code: errorCodeCommandFailed, Err: err}
	}
	return nil
}

// rehearsePrepareResult is what prepare reports: the new rehearsal's id,
// the copied root dir, the URL of its fakeaws, the hashicorp/aws version
// the lock pins and the lock's warnings, or the refusal. Never the admin
// token.
type rehearsePrepareResult struct {
	Schema     string        `json:"schema"`
	Status     CommandStatus `json:"status"`
	ID         string        `json:"id,omitempty"`
	WorkDir    string        `json:"work_dir,omitempty"`
	FakeawsURL string        `json:"fakeaws_url,omitempty"`
	AWSVersion string        `json:"aws_provider_version,omitempty"`
	Warnings   []string      `json:"warnings,omitempty"`
	Refusal    string        `json:"refusal,omitempty"`
}

func (r rehearsePrepareResult) write(cmd *cobra.Command, mode OutputMode) error {
	out := cmd.OutOrStdout()
	if mode == OutputModeJSON {
		encoded, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return fmt.Errorf("encode prepare result: %w", err)
		}
		_, err = fmt.Fprintf(out, "%s\n", encoded)
		return err
	}
	var human strings.Builder
	fmt.Fprintf(&human, "Command: rehearse prepare\nStatus: %s\n", r.Status)
	if r.Refusal != "" {
		fmt.Fprintf(&human, "Refused: %s\n", r.Refusal)
	} else {
		fmt.Fprintf(&human, "Rehearsal: %s\nWork dir: %s\nFakeaws URL: %s\nAWS provider: hashicorp/aws %s\n", r.ID, r.WorkDir, r.FakeawsURL, r.AWSVersion)
		for _, warning := range r.Warnings {
			fmt.Fprintf(&human, "Warning: %s\n", warning)
		}
	}
	_, err := io.WriteString(out, human.String())
	return err
}
