package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// newRehearsePassthroughCmds is the passthrough family (ADR-0042 §
// Passthrough), one command per rehearsalPassthroughActions entry, each
// run by p: `state mv` sits under a `state` group and the rest under
// rehearse. Like the rehearse group, the state group prints help with no
// args and makes any other subcommand a usage error, which cobra would
// exit 0 for.
func newRehearsePassthroughCmds(p rehearsePassthrough) []*cobra.Command {
	state := &cobra.Command{
		Use:   "state",
		Short: "Run a tofu state action in a rehearsal",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return rehearseUsage(cmd, fmt.Errorf("unknown command %q: %s", "state "+args[0], rehearsalPassthroughAllowed()))
		},
	}
	cmds := []*cobra.Command{state}
	for _, action := range slices.Sorted(maps.Keys(rehearsalPassthroughActions)) {
		if strings.HasPrefix(action, "state ") {
			state.AddCommand(newRehearsePassthroughCmd(p, action))
			continue
		}
		cmds = append(cmds, newRehearsePassthroughCmd(p, action))
	}
	return cmds
}

// newRehearsePassthroughCmd is `rehearse <action> <id> [-- tofu args]`.
// It sets no persistent pre-run: the group's makes a flag error a usage
// error.
func newRehearsePassthroughCmd(p rehearsePassthrough, action string) *cobra.Command {
	words := strings.Fields(action)
	return &cobra.Command{
		Use:   words[len(words)-1] + " <id> [-- tofu args]",
		Short: fmt.Sprintf("Run tofu %s in a rehearsal's root against its private fakeaws; tofu's own args go after --", action),
		Args: func(cmd *cobra.Command, args []string) error {
			before := len(args)
			if at := cmd.ArgsLenAtDash(); at >= 0 {
				before = at
			}
			if before != 1 {
				return rehearseUsage(cmd, fmt.Errorf("takes exactly one <id> before --, got %d", before))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return p.runCmd(cmd, action, args)
		},
	}
}

// runCmd runs action for `<id> [-- tofu args]`. --output json is a usage
// error before tofu runs: tofu writes its own output, and in human mode
// nothing is printed after it exits.
func (p rehearsePassthrough) runCmd(cmd *cobra.Command, action string, args []string) error {
	mode, err := outputModeFromCommand(cmd)
	if err != nil {
		return err
	}
	if mode == OutputModeJSON {
		return rehearseUsage(cmd, fmt.Errorf("--output json is refused: tofu %s writes its own output", action))
	}
	p.stdout, p.stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := p.run(cmd.Context(), args[0], action, args[1:]); err != nil {
		return &CLIError{Op: cmd.CommandPath(), Code: errorCodeCommandFailed, Err: err}
	}
	return nil
}
