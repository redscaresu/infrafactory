package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// avoidCheckDetailLimit caps the failure text a ledger record keeps. The
// ledger is committed; the whole stderr stays in the operator's terminal.
const avoidCheckDetailLimit = 1000

func newPitfallsCheckAvoidCmd(cfg *rootConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-avoid <cloud>",
		Short: "Replay the shape a mock-learned avoid rule forbids against the cloud's mock, and retire the rule if it applies cleanly",
		Args:  cobra.ExactArgs(1),
		RunE:  cfg.withRuntimeNoGenerator("pitfalls check-avoid", runPitfallsCheckAvoidCommand),
	}
	cmd.Flags().String("resource", "", "Resource type the avoid rule names, e.g. aws_subnet")
	cmd.Flags().StringSlice("attribute", nil, "Attribute the rule forbids; repeat for every attribute of the rule")
	cmd.Flags().String("from", "", "Directory of .tf files to cut the forbidden shape from, e.g. a run's iterations/<n>/generated")
	for _, name := range []string{"resource", "attribute", "from"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

// avoidCheckRequest is what the operator asked to check.
type avoidCheckRequest struct {
	Resource   string
	Attributes []string
	From       string
}

// runPitfallsCheckAvoidCommand retires a `source: avoid` rule only on
// evidence that contradicts it (ADR-0034): the shape it forbids, cut from
// real generated HCL, applies and converges on the cloud's mock. No LLM
// is involved, so the evidence is deterministic and the command costs
// nothing to run.
func runPitfallsCheckAvoidCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	const op = "pitfalls check-avoid"
	cloud := args[0]
	var req avoidCheckRequest
	var err error
	if req.Resource, err = cmd.Flags().GetString("resource"); err != nil {
		return &CLIError{Op: op, Code: errorCodeUsage, Err: err}
	}
	if req.Attributes, err = cmd.Flags().GetStringSlice("attribute"); err != nil {
		return &CLIError{Op: op, Code: errorCodeUsage, Err: err}
	}
	if req.From, err = cmd.Flags().GetString("from"); err != nil {
		return &CLIError{Op: op, Code: errorCodeUsage, Err: err}
	}

	replay, err := newAvoidReplay(runtime.Config, cloud, execCommandRunner{}, nil)
	if err != nil {
		return &CLIError{Op: op, Code: errorCodeConfigInvalid, Err: fmt.Errorf("refusing: %w", err)}
	}
	pitfallsDir := runtime.Config.Paths.Pitfalls
	rec, err := checkAvoid(cmd.Context(), pitfallsDir, cloud, req, replay, time.Now())
	if err != nil {
		return &CLIError{Op: op, Code: errorCodeCommandFailed, Err: err}
	}
	return reportAvoidCheck(cmd, pitfallsDir, cloud, rec)
}

// checkAvoid refuses before any tofu call unless the check can retire
// the rule, then stores the cut, replays it and records the outcome.
func checkAvoid(ctx context.Context, pitfallsDir, cloud string, req avoidCheckRequest, replay *avoidReplay, now time.Time) (generator.AvoidLedgerRecord, error) {
	prep, err := generator.PrepareAvoidCheck(pitfallsDir, cloud, req.From, req.Resource, req.Attributes)
	if err != nil {
		return generator.AvoidLedgerRecord{}, fmt.Errorf("refusing: %w", err)
	}
	checkID := now.UTC().Format("20060102T150405Z") + "-" + req.Resource
	shapeDir, sum, err := generator.WriteAvoidShape(pitfallsDir, checkID, prep.Shape)
	if err != nil {
		return generator.AvoidLedgerRecord{}, err
	}
	result, err := replay.run(ctx, shapeDir, req.Resource, req.Attributes)
	if err != nil {
		_ = os.RemoveAll(shapeDir)
		return generator.AvoidLedgerRecord{}, fmt.Errorf("the check did not run against the mock, nothing recorded: %w", err)
	}

	retirement := prep.Retirement
	retirement.Check = generator.AvoidCheck{
		ID:          checkID,
		At:          now.UTC().Format(time.RFC3339),
		From:        recordedFrom(req.From),
		ShapeSHA256: sum,
		ApplyExit:   &result.ApplyExit,
		PlanExit:    &result.PlanExit,
		Outcome:     result.Outcome,
		Detail:      result.Detail,
	}
	rec, err := generator.RetireAvoidPitfall(pitfallsDir, cloud, retirement)
	if err != nil {
		_ = os.RemoveAll(shapeDir)
		return generator.AvoidLedgerRecord{}, err
	}
	return rec, nil
}

// recordedFrom keeps a run path from .infrafactory/runs on, so the
// committed ledger does not carry the operator's home directory.
func recordedFrom(from string) string {
	from = filepath.ToSlash(filepath.Clean(from))
	if i := strings.Index(from, ".infrafactory/runs/"); i >= 0 {
		return from[i:]
	}
	return from
}

// AvoidReplayResult is one replay of a stored shape against the mock.
type AvoidReplayResult struct {
	ApplyExit int
	PlanExit  int
	Outcome   string
	Detail    string
}

// avoidReplay deploys a stored shape on one cloud's mock at Layer 2.
type avoidReplay struct {
	cfg    config.Config
	deploy *harness.MockDeployHarness
	env    map[string]string
}

// newAvoidReplay is the one constructor for a replay: check-avoid and
// CI's replay of retired records (ReplayAvoidShape) both build theirs
// here. It deploys over the cloud's own mock client, never
// cloudMockStateRouter, which with no scenario loaded falls back to
// mockway. Only aws is wired. Layer 3 is never constructed.
func newAvoidReplay(cfg config.Config, cloud string, runner harness.CommandRunner, mock harness.MockStateClient) (*avoidReplay, error) {
	if cloud != "aws" {
		return nil, fmt.Errorf("cloud %q has no wired mock to replay against; only aws does", cloud)
	}
	mockURL := strings.TrimSpace(cfg.Fakeaws.URL)
	if mockURL == "" {
		return nil, errors.New("fakeaws.url is not set: aws has no mock to replay against")
	}
	if mock == nil {
		mock = newMockStateClient(mockURL)
	}
	return &avoidReplay{cfg: cfg, deploy: harness.NewMockDeployHarness(runner, mock), env: awsLayer2Env(cfg)}, nil
}

// ReplayAvoidShape replays a stored shape against cloud's mock exactly as
// check-avoid does.
func ReplayAvoidShape(ctx context.Context, cfg config.Config, cloud, shapeDir, resource string, attributes []string) (AvoidReplayResult, error) {
	replay, err := newAvoidReplay(cfg, cloud, execCommandRunner{}, nil)
	if err != nil {
		return AvoidReplayResult{}, err
	}
	return replay.run(ctx, shapeDir, resource, attributes)
}

// run applies the shape with infrafactory's own provider block (runID
// "": no default_tags) from a scratch dir, then classifies. A reset,
// init or state failure is an error: the check never answered.
func (r *avoidReplay) run(ctx context.Context, shapeDir, resource string, attrs []string) (AvoidReplayResult, error) {
	files, err := readShapeFiles(shapeDir)
	if err != nil {
		return AvoidReplayResult{}, err
	}
	if err := ensureAwsProviderWiring(files, r.cfg, ""); err != nil {
		return AvoidReplayResult{}, err
	}
	work, err := os.MkdirTemp("", "infrafactory-check-avoid-")
	if err != nil {
		return AvoidReplayResult{}, err
	}
	defer os.RemoveAll(work)
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(work, name), content, 0o644); err != nil {
			return AvoidReplayResult{}, err
		}
	}

	res, err := r.deploy.Run(ctx, work, r.env, harness.MockDeployModeClean)
	var deployErr *harness.MockDeployError
	switch {
	case err == nil && !res.Drifted:
		return AvoidReplayResult{ApplyExit: 0, PlanExit: 0, Outcome: generator.AvoidOutcomeContradicted}, nil
	case err == nil:
		return failedReplay(0, 2, "converge plan not empty: "+res.Converge.Stdout, resource, attrs), nil
	case errors.As(err, &deployErr) && deployErr.Stage == "apply":
		// tofu apply exits 1 on any error.
		return failedReplay(1, generator.AvoidExitNotRun, deployErr.Apply.Stderr, resource, attrs), nil
	case errors.As(err, &deployErr) && deployErr.Stage == "converge":
		return failedReplay(0, 1, deployErr.Converge.Stderr, resource, attrs), nil
	default:
		return AvoidReplayResult{}, err
	}
}

func failedReplay(applyExit, planExit int, detail, resource string, attrs []string) AvoidReplayResult {
	outcome := generator.ClassifyAvoidFailure(detail, resource, attrs)
	detail = strings.TrimSpace(detail)
	if len(detail) > avoidCheckDetailLimit {
		// The ledger is published; CutText never leaves a partial id.
		detail = generator.CutText(detail, avoidCheckDetailLimit) + "…"
	}
	return AvoidReplayResult{ApplyExit: applyExit, PlanExit: planExit, Outcome: outcome, Detail: detail}
}

func readShapeFiles(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read shape: %w", err)
	}
	files := map[string][]byte{}
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".tf") {
			return nil, fmt.Errorf("shape %s may hold only regular .tf files, found %q", dir, e.Name())
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		files[e.Name()] = content
	}
	return files, nil
}

// reportAvoidCheck names the record written, and the corpus entry that
// went with it when the rule was retired.
func reportAvoidCheck(cmd *cobra.Command, pitfallsDir, cloud string, rec generator.AvoidLedgerRecord) error {
	c := rec.Check
	stages := []StageSummary{{
		Layer: "pitfalls", Stage: "check-avoid", Status: StageStatusPass,
		Detail: fmt.Sprintf("%s record %s: %s %v, outcome %s (apply_exit %d, plan_exit %d), appended to %s, shape %s",
			rec.Status, c.ID, rec.Resource, rec.Attributes, c.Outcome, *c.ApplyExit, *c.PlanExit,
			filepath.Join(pitfallsDir, "avoid-checks", cloud+".yaml"),
			filepath.Join(pitfallsDir, "avoid-checks", "shapes", c.ID)),
	}}
	if rec.Status == generator.AvoidRecordRetired {
		stages = append(stages, StageSummary{
			Layer: "pitfalls", Stage: "check-avoid", Status: StageStatusPass,
			Detail: fmt.Sprintf("removed from %s: %s", filepath.Join(pitfallsDir, cloud+".yaml"), truncateRule(rec.Entry.Rule)),
		})
	}
	return writeCommandOutput(cmd, OutputResult{
		Command: "pitfalls check-avoid",
		Status:  CommandStatusSuccess,
		Stages:  stages,
	})
}
