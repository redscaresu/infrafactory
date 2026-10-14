package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newRehearseCmd is the parent of the rehearsal family (ADR-0042). It reads
// no infrafactory.yaml: a rehearsal takes its account and region from flags.
//
// Cobra treats none of these as usage on a group: an unknown subcommand
// prints help and exits 0, and a flag parse error, a missing required flag
// or a broken flag group exits 1. So with no args it prints help, any arg is
// an unknown subcommand, and the flag errors are wrapped as usage, exit 2,
// for the whole family: subcommands inherit the flag-error func and the
// persistent pre-run, which validates required flags and flag groups before
// cobra does.
func newRehearseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rehearse",
		Short: "Rehearse your own OpenTofu change against a private fakeaws copy of an AWS account (ADR-0042)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return rehearseUsage(cmd, fmt.Errorf("unknown command %q", args[0]))
		},
	}
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		// Cobra runs only the nearest persistent pre-run, so run the root's
		// (the --output check) first.
		if parent := cmd.Parent(); parent != nil && parent.PersistentPreRunE != nil {
			if err := parent.PersistentPreRunE(c, args); err != nil {
				return err
			}
		}
		if err := c.ValidateRequiredFlags(); err != nil {
			return rehearseUsage(c, err)
		}
		if err := c.ValidateFlagGroups(); err != nil {
			return rehearseUsage(c, err)
		}
		return nil
	}
	cmd.SetFlagErrorFunc(rehearseUsage)
	cmd.AddCommand(newRehearsePrepareCmd(rehearsePrepare{}), newRehearseReadCmd(rehearseRead{}), newRehearseEndCmd(rehearseEnd{}))
	return cmd
}

func rehearseUsage(cmd *cobra.Command, err error) error {
	return &CLIError{Op: cmd.CommandPath(), Code: errorCodeUsage, Err: err}
}
