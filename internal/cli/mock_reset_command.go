package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// runMockResetCommand resets state across every configured mock
// backend in one call: mockway (Scaleway), fakegcp, fakeaws, and the
// s3 carve-out (SeaweedFS by default). Mirrors the cloudMockStateRouter
// fan-out used by `infrafactory run`'s clean-deploy reset, but
// scenario-independent so sweep harnesses can drop pre-sweep state
// without a SeaweedFS-specific curl carve-out.
//
// Closes the S54 sustain-ratchet gap: bare-curl `/mock/reset` to
// fakeaws does not cascade to SeaweedFS, so prior session state leaks
// into sweeps as `BucketAlreadyExists`.
func runMockResetCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	ctx := cmd.Context()
	cfg := runtime.Config

	router := &cloudMockStateRouter{
		runtime:  runtime,
		scaleway: newMockStateClient(cfg.Mockway.URL),
	}
	if strings.TrimSpace(cfg.Fakegcp.URL) != "" {
		router.gcp = newMockStateClient(cfg.Fakegcp.URL)
	}
	if strings.TrimSpace(cfg.Fakeaws.URL) != "" {
		router.aws = newMockStateClient(cfg.Fakeaws.URL)
	}
	if strings.TrimSpace(cfg.S3.URL) != "" {
		router.s3 = newMockStateClient(cfg.S3.URL)
	}

	resetErr := router.ResetAll(ctx)

	stages := []StageSummary{
		{Layer: "mock", Stage: "reset", Status: StageStatusPass, Detail: resetSummary(router)},
	}
	failures := []FailureSummary{}
	status := CommandStatusSuccess
	if resetErr != nil {
		stages[0].Status = StageStatusFail
		status = CommandStatusFailed
		failures = append(failures, FailureSummary{
			Layer:   "mock",
			Stage:   "reset",
			Check:   "reset",
			Command: "mock reset",
			Detail:  resetErr.Error(),
		})
	}

	result := OutputResult{
		Command:  "mock reset",
		Scenario: "n/a",
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}
	if outErr := writeCommandOutput(cmd, result); outErr != nil {
		return outErr
	}
	if status == CommandStatusFailed {
		return &CLIError{Op: "mock reset", Code: errorCodeCommandFailed, Err: fmt.Errorf("mock reset failed")}
	}
	return nil
}

// resetSummary describes which backends were touched, in the same
// order ResetAll calls them. Useful for the human/JSON output so
// operators can tell at a glance whether s3 cascaded.
func resetSummary(r *cloudMockStateRouter) string {
	parts := []string{"mockway"}
	if r.gcp != nil {
		parts = append(parts, "fakegcp")
	}
	if r.aws != nil {
		parts = append(parts, "fakeaws")
	}
	if r.s3 != nil {
		parts = append(parts, "s3")
	}
	return "reset " + strings.Join(parts, "+")
}
