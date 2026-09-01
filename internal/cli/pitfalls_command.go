package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/generator"
)

// newPitfallsCmd groups operations on the learned corpus itself, as
// opposed to the runs that produce it.
func newPitfallsCmd(cfg *rootConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pitfalls",
		Short: "Inspect and maintain the learned pitfall corpus",
	}

	retire := &cobra.Command{
		Use:   "retire <cloud>",
		Short: "Remove live-sourced pitfalls that have not been seen for a while",
		Args:  cobra.ExactArgs(1),
		RunE:  cfg.withRuntimeNoGenerator("pitfalls retire", runPitfallsRetireCommand),
	}
	retire.Flags().Duration("older-than", generator.DefaultLiveRetention,
		"Retire live pitfalls last seen longer ago than this")
	retire.Flags().Bool("dry-run", false, "Report what would be retired without changing the corpus")
	cmd.AddCommand(retire)

	return cmd
}

// runPitfallsRetireCommand is the corpus's only outflow (S156a).
//
// Every other part of the learning loop adds entries. Learning used to be
// bounded -- a run emits at most `repair_iterations_max` failures, and a
// scenario that stops failing stops emitting -- and live observation
// removed that bound. Nothing else can take an entry out once the cause
// behind it is fixed.
//
// A stale pitfall is not inert. It steers generation away from something
// that is no longer broken, which makes every future generation worse
// **silently**, and silent degradation is the failure mode this project
// has learned to fear most.
func runPitfallsRetireCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	cloud := args[0]

	olderThan, err := cmd.Flags().GetDuration("older-than")
	if err != nil {
		return &CLIError{Op: "pitfalls retire", Code: errorCodeUsage, Err: err}
	}
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return &CLIError{Op: "pitfalls retire", Code: errorCodeUsage, Err: err}
	}

	dir := runtime.Config.Paths.Pitfalls
	now := time.Now()

	if dryRun {
		// Reads the corpus and reports, changing nothing. Worth having
		// because retirement deletes learning, and an operator should be
		// able to see what a threshold would remove before it removes it.
		stale, err := generator.StaleLivePitfalls(dir, cloud, olderThan, now)
		if err != nil {
			return &CLIError{Op: "pitfalls retire", Code: errorCodeCommandFailed, Err: err}
		}
		return reportRetired(cmd, cloud, stale, true)
	}

	retired, err := generator.RetireStaleLivePitfalls(dir, cloud, olderThan, now)
	if err != nil {
		return &CLIError{Op: "pitfalls retire", Code: errorCodeCommandFailed, Err: err}
	}
	return reportRetired(cmd, cloud, retired, false)
}

// reportRetired names every entry that went, because a corpus that
// quietly drops entries is indistinguishable from one that never learned
// them -- the rule the D6 purge established.
func reportRetired(cmd *cobra.Command, cloud string, retired []generator.RetiredPitfall, dryRun bool) error {
	stages := []StageSummary{{
		Layer: "pitfalls", Stage: "retire", Status: StageStatusPass,
		Detail: retirementSummary(cloud, retired, dryRun),
	}}

	for _, entry := range retired {
		stages = append(stages, StageSummary{
			Layer: "pitfalls", Stage: "retire", Status: StageStatusPass,
			Detail: fmt.Sprintf("%s: last seen %s (%s ago) — %s",
				entry.Resource,
				entry.LastSeen.UTC().Format(time.RFC3339),
				entry.Age.Round(time.Hour),
				truncateRule(entry.Rule)),
		})
	}

	return writeCommandOutput(cmd, OutputResult{
		Command: "pitfalls retire",
		Status:  CommandStatusSuccess,
		Stages:  stages,
	})
}

func retirementSummary(cloud string, retired []generator.RetiredPitfall, dryRun bool) string {
	if len(retired) == 0 {
		return fmt.Sprintf("no live pitfalls in %s have gone stale", cloud)
	}
	if dryRun {
		return fmt.Sprintf("--dry-run: %d live pitfall(s) in %s would be retired", len(retired), cloud)
	}
	return fmt.Sprintf("retired %d live pitfall(s) from %s", len(retired), cloud)
}

// truncateRule keeps a summary line readable. Rules can be whole HCL
// snippets, and a stage list that scrolls for a page is one nobody reads.
func truncateRule(rule string) string {
	const limit = 100
	flat := ""
	for _, r := range rule {
		if r == '\n' || r == '\t' {
			r = ' '
		}
		flat += string(r)
		if len(flat) >= limit {
			return flat + "…"
		}
	}
	return flat
}
