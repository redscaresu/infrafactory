package cli

import (
	"encoding/json"
	"fmt"
	"os"
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
	cmd.Flags().String("var-file", "", "a .tfvars file, copied into the rehearsal as its only variable source")
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
	mode, err := outputModeFromCommand(cmd)
	if err != nil {
		return err
	}
	if p.wd == "" {
		if p.wd, err = os.Getwd(); err != nil {
			return &CLIError{Op: cmd.CommandPath(), Code: errorCodeCommandFailed, Err: err}
		}
	}

	result := rehearsePrepareResult{Schema: rehearsePrepareSchema, Status: CommandStatusSuccess}
	result.ID, result.WorkDir, err = p.prepare(args[0], region, varFile)
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

// rehearsePrepareResult is what prepare reports: the new rehearsal's id and
// the copied root dir, or the refusal.
type rehearsePrepareResult struct {
	Schema  string        `json:"schema"`
	Status  CommandStatus `json:"status"`
	ID      string        `json:"id,omitempty"`
	WorkDir string        `json:"work_dir,omitempty"`
	Refusal string        `json:"refusal,omitempty"`
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
	_, _ = fmt.Fprintf(out, "Command: rehearse prepare\nStatus: %s\n", r.Status)
	if r.Refusal != "" {
		_, err := fmt.Fprintf(out, "Refused: %s\n", r.Refusal)
		return err
	}
	_, err := fmt.Fprintf(out, "Rehearsal: %s\nWork dir: %s\n", r.ID, r.WorkDir)
	return err
}
