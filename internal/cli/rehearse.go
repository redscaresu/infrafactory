package cli

import "github.com/spf13/cobra"

// newRehearseCmd is the parent of the rehearsal family (ADR-0042). It has no
// RunE, so with no subcommand cobra prints its help and exits 0, and it reads
// no infrafactory.yaml: a rehearsal takes its account and region from flags.
func newRehearseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rehearse",
		Short: "Rehearse your own OpenTofu change against a private fakeaws copy of an AWS account (ADR-0042)",
	}
}
