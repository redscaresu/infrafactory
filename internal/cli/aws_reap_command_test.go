package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

const (
	strayKeyPairs = `<keySet><item><keyPairId>key-0a</keyPairId><keyName>a</keyName></item>` +
		`<item><keyPairId>key-0b</keyPairId><keyName>b</keyName></item></keySet>`
	sweepKeyPairs  = "ec2:DescribeKeyPairs"
	deleteKeyPair  = "ec2:DeleteKeyPair"
	getStamp       = "ssm:GetParameter " + harness.AWSStampParameter
	callerIdentity = "sts:GetCallerIdentity"
)

// awsReapRun is one `infrafactory reap` of an aws scenario whose workdir
// holds a stale Scaleway marker, which aws must never read.
type awsReapRun struct {
	h      *CommandTestHarness
	output string
	err    error
}

func (r awsReapRun) text() string {
	if r.err == nil {
		return r.output
	}
	return r.output + r.err.Error()
}

func reapAWS(t *testing.T, lc *awsLifecycle, customize func(*config.Config), withState bool, flags ...string) awsReapRun {
	t.Helper()
	h := newCommandTestHarness(t)
	setScenarioCloud(t, h.ScenarioPath, "aws")
	outDir := filepath.Join(h.OutputDir(), "example-scenario")
	require.NoError(t, os.MkdirAll(outDir, 0o755))
	writeAWSStateAndStaleMarker(t, outDir, withState)

	cfg, err := config.Load(h.ConfigPath)
	require.NoError(t, err)
	cfg.Paths.Output = h.OutputDir()
	cfg = layer3On(cfg)
	cfg.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
	if customize != nil {
		customize(&cfg)
	}
	rt := &CommandRuntime{ConfigPath: h.ConfigPath, Config: cfg, scenarioLoader: defaultScenarioLoader, Logger: NewAppLogger(&bytes.Buffer{})}
	lc.scw.install(&rt.Deps)
	rt.Deps.SandboxDestroy = lc.destroy
	rt.Deps.AWSSTS, rt.Deps.AWSSSM, rt.Deps.AWSEC2 = lc, lc, lc
	rt.Deps.AWSSweepSleep = func(context.Context, time.Duration) error { return nil }

	out := &strings.Builder{}
	err = runReap(t, rt, h.ScenarioPath, out, flags...)
	return awsReapRun{h: h, output: out.String(), err: err}
}

// deleteKeyPairsOnDelete makes DeleteKeyPair empty the key pair listing.
func deleteKeyPairsOnDelete(lc *awsLifecycle) {
	lc.onEC2 = func(action string) {
		if action == "DeleteKeyPair" {
			delete(lc.ec2, "DescribeKeyPairs")
		}
	}
}

// writes is every call that changes the scope: an SSM put or delete, or
// any EC2 action but a Describe.
func writes(lc *awsLifecycle) []string {
	var found []string
	for _, call := range lc.log() {
		if strings.HasPrefix(call, "ssm:PutParameter") || strings.HasPrefix(call, "ssm:DeleteParameter") ||
			(strings.HasPrefix(call, "ec2:") && !strings.HasPrefix(call, "ec2:Describe")) {
			found = append(found, call)
		}
	}
	return found
}

func lastIndex(calls []string, call string) int {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i] == call {
			return i
		}
	}
	return -1
}

func TestAWSReapClaimsSweepsDeletesAndReleases(t *testing.T) {
	for name, withState := range map[string]bool{"no state, no marker read": false, "state": true} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			lc.ec2["DescribeKeyPairs"] = strayKeyPairs
			deleteKeyPairsOnDelete(lc)

			r := reapAWS(t, lc, nil, withState)

			require.NoError(t, r.err, r.output)
			calls := lc.log()
			order := []int{
				slices.Index(calls, callerIdentity),
				slices.Index(calls, getStamp),
				slices.Index(calls, putClaim),
				slices.Index(calls, sweepKeyPairs),
				slices.Index(calls, deleteKeyPair),
				lastIndex(calls, sweepKeyPairs),
				slices.Index(calls, deleteClaim),
			}
			assert.NotContains(t, order, -1, "every step ran: %v", calls)
			assert.True(t, slices.IsSorted(order),
				"GetCallerIdentity, stamp, TakeAWSClaim, sweep, delete, verdict sweep, DeleteParameter: %v", calls)
			assert.Equal(t, 2, lc.count(deleteKeyPair), "both strays deleted")
			assert.Equal(t, 1, lc.count(deleteClaim), "released once")
			_, held := lc.claim()
			assert.False(t, held, "released")
			assert.Contains(t, lc.params, harness.AWSStampParameter, "the stamp stays")

			if withState {
				destroy := slices.Index(calls, destroyRun)
				assert.Less(t, slices.Index(calls, putClaim), destroy, "the destroy after the claim: %v", calls)
				assert.Less(t, destroy, slices.Index(calls, sweepKeyPairs), "the destroy before the sweep: %v", calls)
			} else {
				assert.Zero(t, lc.destroy.calls, "no state, nothing for tofu to destroy")
			}
			lc.scw.assertUntouched(t)
		})
	}
}

func TestAWSReapRefusesAWrongAccountOrStampWithNoWrites(t *testing.T) {
	for name, tc := range map[string]struct {
		customize func(*config.Config)
		setup     func(*awsLifecycle)
		want      string
	}{
		"wrong account": {
			customize: func(cfg *config.Config) {
				cfg.AWS.AccountID = "210987654321"
				cfg.AWS.PrincipalARN = "arn:aws:iam::210987654321:user/infrafactory-layer3"
			},
			want: "210987654321",
		},
		"stamp names another account": {
			setup: func(lc *awsLifecycle) { lc.params[harness.AWSStampParameter] = "210987654321" },
			want:  `holds "210987654321"`,
		},
		"no stamp": {
			setup: func(lc *awsLifecycle) { delete(lc.params, harness.AWSStampParameter) },
			want:  harness.AWSStampParameter + " does not exist",
		},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			lc.ec2["DescribeKeyPairs"] = strayKeyPairs
			if tc.setup != nil {
				tc.setup(lc)
			}

			r := reapAWS(t, lc, tc.customize, true)

			require.Error(t, r.err)
			assert.Contains(t, r.err.Error(), tc.want)
			assert.Empty(t, writes(lc), "no SSM write and no EC2 mutation")
			assert.Zero(t, lc.destroy.calls, "SandboxDestroy")
			lc.scw.assertUntouched(t)
		})
	}
}

func TestAWSReapRefusesAHeldClaimUnlessTakeOverNamesItsHolder(t *testing.T) {
	t.Run("no --take-over", func(t *testing.T) {
		lc := newAWSLifecycle(t)
		lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder

		r := reapAWS(t, lc, nil, true)

		require.Error(t, r.err)
		assert.Contains(t, r.err.Error(), reapCommand(r.h.ConfigPath, r.h.ScenarioPath)+" --take-over "+lifecycleOtherHolder)
		assert.Empty(t, writes(lc))
		assert.Zero(t, lc.destroy.calls, "SandboxDestroy")
		holder, _ := lc.claim()
		assert.Equal(t, lifecycleOtherHolder, holder)
	})

	t.Run("--take-over names someone else", func(t *testing.T) {
		lc := newAWSLifecycle(t)
		lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder

		r := reapAWS(t, lc, nil, true, "--take-over", "run-1@third-host.example:7")

		require.Error(t, r.err)
		assert.Contains(t, r.err.Error(), `by "`+lifecycleOtherHolder+`"`)
		assert.Empty(t, writes(lc))
		holder, _ := lc.claim()
		assert.Equal(t, lifecycleOtherHolder, holder)
	})

	t.Run("--take-over names the holder", func(t *testing.T) {
		lc := newAWSLifecycle(t)
		lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder

		r := reapAWS(t, lc, nil, false, "--take-over", lifecycleOtherHolder)

		require.NoError(t, r.err, r.output)
		calls := lc.log()
		assert.Less(t, slices.Index(calls, deleteClaim), slices.Index(calls, putClaim), "the old claim goes before ours: %v", calls)
		assert.Less(t, slices.Index(calls, putClaim), slices.Index(calls, sweepKeyPairs), "claimed before the sweep: %v", calls)
		_, held := lc.claim()
		assert.False(t, held, "the scope was empty, so reap released it")
	})
}

func TestAWSReapDirtyVerdictKeepsTheClaim(t *testing.T) {
	lc := newAWSLifecycle(t)
	// Every delete answers success, and the key pairs are still listed.
	lc.ec2["DescribeKeyPairs"] = strayKeyPairs

	r := reapAWS(t, lc, nil, false)

	require.Error(t, r.err)
	for _, stray := range []string{"key pairs key-0a", "key pairs key-0b"} {
		assert.Contains(t, r.output, stray)
	}
	assert.Equal(t, 2, lc.count(deleteKeyPair))
	assert.Zero(t, lc.count(deleteClaim), "DeleteParameter")
	holder, held := lc.claim()
	require.True(t, held, "the claim is kept")
	assert.True(t, strings.HasPrefix(holder, awsReapHolderPrefix), "held by the reap: %s", holder)
	assert.Contains(t, r.output, reapCommand(r.h.ConfigPath, r.h.ScenarioPath)+" --take-over "+holder)
}

// A failed destroy fails the reap, and the verdict still decides the
// claim: an empty scope releases it, and nothing says it was kept.
func TestAWSReapFailedDestroyStillReleasesAnEmptyScope(t *testing.T) {
	lc := newAWSLifecycle(t)
	lc.destroy.err = errors.New("tofu destroy failed")

	r := reapAWS(t, lc, nil, true)

	require.Error(t, r.err)
	assert.Contains(t, r.output, "tofu destroy failed")
	assert.Equal(t, 1, lc.count(deleteClaim), "released")
	_, held := lc.claim()
	assert.False(t, held)
	assert.NotContains(t, r.output, "keeps the aws scope's claim")
}

func TestAWSReapDryRunWritesNothing(t *testing.T) {
	t.Run("a stray", func(t *testing.T) {
		lc := newAWSLifecycle(t)
		lc.ec2["DescribeKeyPairs"] = strayKeyPairs
		lc.ec2["DescribeInstances"] = runningInstance

		r := reapAWS(t, lc, nil, true, "--dry-run")

		require.Error(t, r.err)
		for _, stray := range []string{"key pairs key-0a", "key pairs key-0b", "instances i-0stray"} {
			assert.Contains(t, r.text(), stray)
		}
		assert.Empty(t, writes(lc), "no Put, Delete, Terminate, Release or Revoke")
		assert.Zero(t, lc.destroy.calls, "SandboxDestroy")
		lc.scw.assertUntouched(t)
	})

	t.Run("an empty scope", func(t *testing.T) {
		lc := newAWSLifecycle(t)

		r := reapAWS(t, lc, nil, true, "--dry-run")

		require.NoError(t, r.err)
		assert.Contains(t, r.output, "nothing to reap")
		assert.Empty(t, writes(lc))
	})
}

// The interrupt print for each thing a run may know of the claim. Each
// reap form refuses the other's case, so only the one that works is
// named, and both only when the run cannot tell.
func assertInterruptNamesTheReap(t *testing.T, lc *awsLifecycle, output, configPath, scenarioPath string, state awsClaimState) {
	t.Helper()
	plain := "`" + reapCommand(configPath, scenarioPath) + "`"
	takeOver := "`" + lc.takeOver(configPath, scenarioPath) + "`"
	// The notice alone: a kept claim's stage detail elsewhere in the
	// output names --take-over on its own account.
	_, notice, found := strings.Cut(output, "\nInterrupted: ")
	require.True(t, found, "no interrupt notice: %s", output)
	notice, _, _ = strings.Cut(notice, "\n")
	switch state {
	case awsClaimHeld:
		assert.True(t, strings.HasPrefix(notice, "this run keeps the aws scope's claim"), notice)
		assert.Contains(t, notice, takeOver+" sweeps the scope")
		assert.NotContains(t, notice, plain)
	case awsClaimUnknown:
		assert.True(t, strings.HasPrefix(notice, "this run may hold the aws scope's claim"), notice)
		assert.Contains(t, notice, "If this run holds the claim, "+takeOver+" sweeps the scope")
		assert.Contains(t, notice, "if no one holds it, "+plain+" does")
	case awsClaimHeldByOther:
		assert.True(t, strings.HasPrefix(notice, "this run does not hold the aws scope's claim: "+lifecycleOtherHolder+" does"), notice)
		assert.Contains(t, notice, "`"+reapCommand(configPath, scenarioPath)+" --take-over "+shellQuote(lifecycleOtherHolder)+"` sweeps the scope")
	default:
		assert.True(t, strings.HasPrefix(notice, "this run does not hold the aws scope's claim: it never took it"), notice)
		assert.Contains(t, notice, plain+" sweeps the scope")
		assert.NotContains(t, notice, "--take-over")
	}
}

// A partial apply that wrote no state: the case where the Scaleway guard
// says there is nothing to clean up. A dirty scope keeps the claim; a
// clean one is released.
func TestInterruptedAWSTestNamesTheReapForAKeptOrReleasedClaim(t *testing.T) {
	for name, state := range map[string]awsClaimState{"dirty scope": awsClaimHeld, "clean scope": awsClaimNotHeld} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			if state == awsClaimHeld {
				lc.ec2["DescribeInstances"] = runningInstance
			}
			lc.deploy.err = context.Canceled
			lc.deploy.onRunDir = func(dir string) {
				lc.record(deployRun)
				writeAWSStateAndStaleMarker(t, dir, false)
				lc.cancel()
			}

			run := runAWSTest(t, lc, nil, nil)

			require.Error(t, run.err)
			assertInterruptNamesTheReap(t, lc, run.output, run.h.ConfigPath, run.h.ScenarioPath, state)
			assert.NotContains(t, run.output, "nothing to clean up")
			_, held := lc.claim()
			assert.Equal(t, state == awsClaimHeld, held, "the claim is kept only for a dirty scope")
			if state == awsClaimHeld {
				assert.Zero(t, lc.count(deleteClaim), "DeleteParameter")
			}
			lc.scw.assertUntouched(t)
		})
	}
}

// Interrupted during the claim's put: a put whose outcome is unknown
// names both reaps, and a claim another run holds names that run.
func TestInterruptedAWSTestNamesTheReapForAnUnknownOrForeignClaim(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(*awsLifecycle)
		state awsClaimState
	}{
		"unknown outcome": {setup: func(lc *awsLifecycle) { lc.putFail = true }, state: awsClaimUnknown},
		"another holder": {
			setup: func(lc *awsLifecycle) { lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder },
			state: awsClaimHeldByOther,
		},
	} {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			tc.setup(lc)
			lc.onPut = func() { lc.cancel() }

			run := runAWSTest(t, lc, nil, nil)

			require.Error(t, run.err)
			assert.Zero(t, lc.deploy.calls, "SandboxDeploy")
			assertInterruptNamesTheReap(t, lc, run.output, run.h.ConfigPath, run.h.ScenarioPath, tc.state)
			lc.scw.assertUntouched(t)
		})
	}
}

// A held or unknown claim with no holder to name, and a runtime that
// never set the claim, hedge: never "keeps the claim" beside plain reap,
// and never one command twice.
func TestAWSReapAdviceHedgesWithoutAHolder(t *testing.T) {
	for name, claim := range map[string]awsClaim{
		"never set":          {},
		"held, no holder":    {state: awsClaimHeld},
		"unknown, no holder": {state: awsClaimUnknown},
		"another, unnamed":   {state: awsClaimHeldByOther},
	} {
		t.Run(name, func(t *testing.T) {
			rt := &CommandRuntime{ConfigPath: config.DefaultPath, scenarioPath: "scenarios/training/aws-web-live.yaml", awsClaim: claim}
			plain := "`" + reapCommand(rt.ConfigPath, rt.scenarioPath) + "`"

			notice := awsInterruptNotice(rt)

			assert.Contains(t, notice, "Interrupted: this run may hold the aws scope's claim")
			assert.Contains(t, notice, "The claim's holder is unknown here")
			assert.NotContains(t, notice, "keeps the aws scope's claim")
			assert.NotContains(t, notice, "--take-over '")
			assert.Equal(t, 1, strings.Count(notice, plain), notice)
		})
	}
}

// A release clears the holder with the state; a failed one is unknown,
// unless it names the holder that has the claim.
func TestAWSClaimAfterRelease(t *testing.T) {
	const holder = "run-9@this-host.example:1"
	for name, tc := range map[string]struct {
		err  error
		want awsClaim
	}{
		"released":       {want: awsClaim{state: awsClaimNotHeld}},
		"no claim":       {err: fmt.Errorf("refusing: %w", harness.ErrAWSNoClaimHeld), want: awsClaim{state: awsClaimNotHeld}},
		"response lost":  {err: errors.New("ssm:DeleteParameter: connection reset"), want: awsClaim{holder: holder, state: awsClaimUnknown}},
		"another holder": {err: fmt.Errorf("refusing: %w", &harness.AWSScopeClaimedError{Holder: lifecycleOtherHolder}), want: awsClaim{holder: holder, state: awsClaimHeldByOther, other: lifecycleOtherHolder}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, awsClaimAfterRelease(holder, tc.err))
		})
	}
}

// A release that fails after a clean sweep cannot say the claim is still
// this run's: the delete may have landed, or someone else may hold it.
func TestAWSTestNamesTheReapAfterAFailedRelease(t *testing.T) {
	t.Run("delete denied", func(t *testing.T) {
		lc := newAWSLifecycle(t)
		lc.denied[deleteClaim] = true

		run := runAWSTest(t, lc, nil, nil)

		require.Error(t, run.err)
		details := run.failureDetails()
		assert.Contains(t, details, "this run may hold the aws scope's claim for "+lc.runHolder)
		assert.Contains(t, details, "If this run holds the claim, `"+lc.takeOver(run.h.ConfigPath, run.h.ScenarioPath)+"`")
		assert.Contains(t, details, "if no one holds it, `"+reapCommand(run.h.ConfigPath, run.h.ScenarioPath)+"` does")
	})
	t.Run("the claim is gone", func(t *testing.T) {
		lc := newAWSLifecycle(t)
		lc.onEC2 = func(action string) {
			if action == "DescribeInstances" {
				delete(lc.params, harness.AWSClaimParameter)
			}
		}

		run := runAWSTest(t, lc, nil, nil)

		assert.NotContains(t, run.output, "--take-over", "no claim to take over")
		assert.NotContains(t, run.output, "this run may hold the aws scope's claim")
	})
	t.Run("another holder took it", func(t *testing.T) {
		lc := newAWSLifecycle(t)
		lc.onEC2 = func(action string) {
			if action == "DescribeInstances" {
				lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder
			}
		}

		run := runAWSTest(t, lc, nil, nil)

		require.Error(t, run.err)
		details := run.failureDetails()
		assert.Contains(t, details, "this run does not hold the aws scope's claim: "+lifecycleOtherHolder+" does")
		assert.Contains(t, details, "`"+reapCommand(run.h.ConfigPath, run.h.ScenarioPath)+" --take-over "+shellQuote(lifecycleOtherHolder)+"`")
		assert.NotContains(t, details, "--take-over "+shellQuote(lc.runHolder))
	})
}

// A take-over that deleted the old claim and then failed its own take can
// never be retried as typed: it names the reap that can work.
func TestAWSReapTakeOverNamesTheNextReapWhenItsOwnTakeFails(t *testing.T) {
	lc := newAWSLifecycle(t)
	lc.params[harness.AWSClaimParameter] = lifecycleOtherHolder
	lc.putFail = true

	r := reapAWS(t, lc, nil, false, "--take-over", lifecycleOtherHolder)

	require.Error(t, r.err)
	assert.ErrorIs(t, r.err, harness.ErrAWSPreviousClaimDeleted)
	assert.Contains(t, r.err.Error(), "if no one holds it, `"+reapCommand(r.h.ConfigPath, r.h.ScenarioPath)+"` does")
	assert.Zero(t, lc.destroy.calls, "SandboxDestroy")
}

// A claim this process already holds, from an earlier iteration of run,
// survives an ensure that returns before writing; any other is not held.
func TestEnsureAWSScopeClaimKeepsAClaimThisProcessHolds(t *testing.T) {
	const holder = "run-9@this-host.example:1"
	for name, tc := range map[string]struct {
		before awsClaim
		want   awsClaimState
	}{
		"held by this process": {before: awsClaim{holder: holder, state: awsClaimHeld}, want: awsClaimHeld},
		"released":             {before: awsClaim{state: awsClaimNotHeld}, want: awsClaimNotHeld},
		"never set":            {want: awsClaimNotHeld},
	} {
		t.Run(name, func(t *testing.T) {
			rt := &CommandRuntime{awsClaim: tc.before} // no aws config: ensure returns before any request

			_, _, failures := ensureAWSScopeClaim(context.Background(), rt, holder)

			require.NotEmpty(t, failures, "ensure returned early")
			assert.Equal(t, awsClaim{holder: holder, state: tc.want}, rt.awsClaim)
		})
	}
}

// A deliberate keep says "on purpose" only of a claim the run is known to
// hold; an unknown one is hedged like every other message.
func TestAWSTestNoDestroyHedgesAnUnknownClaim(t *testing.T) {
	lc := newAWSLifecycle(t)
	lc.putFail = true

	run := runAWSTest(t, lc, nil, nil, "--no-destroy")

	require.Error(t, run.err)
	i := slices.IndexFunc(run.result.Stages, isStage(StageAWSScopeClaimKept))
	require.NotEqual(t, -1, i, "stages carry %s", StageAWSScopeClaimKept)
	detail := run.result.Stages[i].Detail
	assert.NotContains(t, detail, "on purpose")
	assert.Contains(t, detail, "this run may hold the aws scope's claim for "+lc.runHolder)
	assert.Contains(t, detail, "if no one holds it, `"+reapCommand(run.h.ConfigPath, run.h.ScenarioPath)+"` does")
}
