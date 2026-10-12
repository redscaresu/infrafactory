package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newRehearseCmd is the parent of the rehearsal family (ADR-0042). It reads
// no infrafactory.yaml: a rehearsal takes its account and region from flags.
//
// Cobra checks neither args nor flag errors as usage on a group: an unknown
// subcommand would print help and exit 0, and an unknown flag would exit 1.
// So with no args it prints help, any arg is an unknown subcommand, and a
// flag error anywhere in the family (subcommands inherit the func) is a
// usage error, exit 2.
func newRehearseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rehearse",
		Short: "Rehearse your own OpenTofu change against a private fakeaws copy of an AWS account (ADR-0042)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return &CLIError{Op: cmd.CommandPath(), Code: errorCodeUsage, Err: fmt.Errorf("unknown command %q", args[0])}
		},
	}
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return &CLIError{Op: c.CommandPath(), Code: errorCodeUsage, Err: err}
	})
	return cmd
}
