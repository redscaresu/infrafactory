// Command n10extract drives the N10/N13 extractors against a recorded
// run directory to emit a candidate LearnedPitfall snippet on stdout.
//
// The N11 retirement protocol (see ADR-0018) step 2 requires a
// `learned_from_diff` (or `learned_from_diff_avoid`) entry to exist
// before a prompt rule can be retired with pitfall replacement. When
// the organic learn loop hasn't fired for the target pattern (e.g.
// the rule has been keeping the LLM correct, so no failure ever
// recorded), this command lets an operator force-extract a candidate
// pitfall from a recorded run dir + a human-supplied failure detail.
//
// Originally written + removed inline in the 2026-06-02 loop session.
// Promoted to a permanent CLI in S70 so future N11 retirements have
// a stable forced-extract path.
//
// Usage:
//
//	n10extract \
//	  --failed-dir   .infrafactory/runs/<scenario>/<run-id>/iterations/1/generated \
//	  --passing-dir  .infrafactory/runs/<scenario>/<run-id>/iterations/2/generated \
//	  --failure-detail "google_storage_bucket.app_assets has no encryption.default_kms_key_name" \
//	  --failure-resource google_storage_bucket.app_assets \
//	  --cloud gcp \
//	  --scenario gcp-storage \
//	  --mode fix
//
// --run-dir is a shorthand that auto-discovers the failed/passing
// pair from an `.infrafactory/runs/<scenario>/<run-id>/` tree by
// picking the last-failing + first-passing iteration directories.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/redscaresu/infrafactory/internal/generator"
	"gopkg.in/yaml.v3"
)

func main() {
	failedDir := flag.String("failed-dir", "", "directory holding the failing iteration's *.tf files")
	passingDir := flag.String("passing-dir", "", "directory holding the passing iteration's *.tf files")
	runDir := flag.String("run-dir", "", "shorthand: an `.infrafactory/runs/<scenario>/<run-id>/` tree; auto-discovers iter pair from iterations/N/ subdirs")
	failureDetail := flag.String("failure-detail", "", "the failure detail string (terraform stderr, OPA detail, etc.) that the extracted snippet should address")
	failureResource := flag.String("failure-resource", "", "the failing resource address (TYPE.NAME); omit to let the extractor parse from --failure-detail or fall back to type-hint inference")
	cloud := flag.String("cloud", "", `target cloud: "aws" | "gcp" | "scaleway"`)
	scenario := flag.String("scenario", "", "scenario name (for DiscoveredFrom)")
	mode := flag.String("mode", "fix", `extractor mode: "fix" (N10 addition-as-fix) or "avoid" (N13 deletion-as-fix)`)
	timestamp := flag.String("timestamp", "", "optional run timestamp (defaults to empty; only used in extractor logging)")
	flag.Parse()

	if *runDir != "" {
		fd, pd, err := autodiscoverIterPair(*runDir)
		if err != nil {
			fail("--run-dir auto-discovery: %v", err)
		}
		if *failedDir == "" {
			*failedDir = fd
		}
		if *passingDir == "" {
			*passingDir = pd
		}
	}

	if *failedDir == "" || *passingDir == "" {
		fail("need --failed-dir + --passing-dir (or --run-dir shorthand)")
	}
	if *failureDetail == "" {
		fail("need --failure-detail")
	}
	if *cloud == "" {
		fail("need --cloud (aws|gcp|scaleway)")
	}
	if *scenario == "" {
		fail("need --scenario")
	}

	var entry *generator.LearnedPitfall
	var err error
	switch *mode {
	case "fix":
		entry, err = generator.ExtractPrescriptiveFix(*failedDir, *passingDir, *failureDetail, *failureResource, *cloud, *scenario, *timestamp)
	case "avoid":
		entry, err = generator.ExtractPrescriptiveAvoid(*failedDir, *passingDir, *failureDetail, *failureResource, *cloud, *scenario, *timestamp)
	default:
		fail(`--mode must be "fix" or "avoid", got %q`, *mode)
	}
	if err != nil {
		fail("extract: %v", err)
	}
	if entry == nil {
		// Print to stderr so a downstream pipe (e.g. `n10extract ... |
		// yq …`) sees an empty stdout and doesn't get confused.
		fmt.Fprintln(os.Stderr, "no entry: extractor returned nil (no productive diff attributable to the failure)")
		os.Exit(2)
	}

	// Emit a single-element pitfalls-file YAML so the operator can pipe
	// straight into `yq` or append to pitfalls/<cloud>.yaml after
	// review.
	out := map[string]any{
		"provider": *cloud,
		"pitfalls": []map[string]any{
			{
				"resource":        entry.Resource,
				"rule":            entry.Rule,
				"source":          orDefault(entry.Source, "learned"),
				"discovered_from": entry.DiscoveredFrom,
			},
		},
	}
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		fail("encode: %v", err)
	}
	_ = enc.Close()
}

// autodiscoverIterPair walks `<runDir>/iterations/` and returns paths
// for the last failing iteration's `generated/` dir + the first
// passing iteration's `generated/` dir. The convention matches the
// run command's layout (see internal/cli/run_command.go's
// iterationGeneratedDir helper).
//
// Failure = the directory exists for an iteration before the last;
// passing = the highest-numbered iteration directory that exists.
// This is a heuristic; for ambiguous cases pass --failed-dir +
// --passing-dir explicitly.
func autodiscoverIterPair(runDir string) (string, string, error) {
	iters := filepath.Join(runDir, "iterations")
	entries, err := os.ReadDir(iters)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", iters, err)
	}
	var nums []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		nums = append(nums, n)
	}
	if len(nums) < 2 {
		return "", "", fmt.Errorf("found only %d iteration dirs under %s; need at least 2", len(nums), iters)
	}
	sort.Ints(nums)
	failedIter := nums[len(nums)-2]
	passingIter := nums[len(nums)-1]
	failedDir := filepath.Join(iters, strconv.Itoa(failedIter), "generated")
	passingDir := filepath.Join(iters, strconv.Itoa(passingIter), "generated")
	if _, err := os.Stat(failedDir); err != nil {
		return "", "", fmt.Errorf("expected %s: %w", failedDir, err)
	}
	if _, err := os.Stat(passingDir); err != nil {
		return "", "", fmt.Errorf("expected %s: %w", passingDir, err)
	}
	return failedDir, passingDir, nil
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "n10extract: "+format+"\n", args...)
	os.Exit(2)
}
