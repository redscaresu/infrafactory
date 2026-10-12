package generator

// N9 — orphan_check extractor. Classifies post-destroy orphaned
// resources across five sub-shapes (ADR-0016):
//
//	#1 LLM-side soft-delete       → emit LearnedPitfall
//	#2 Mock-side auto-seeded      → emit MockGap
//	#3 Mock-side CASCADE missing  → emit MockGap
//	#4 Mock-vs-provider state     → emit MockGap
//	#5 Provider-side soft-delete  → emit MockGap (provider quirk; mock
//	                                 can usually hard-delete to match)
//
// The single `orphan_check detected N orphaned resources` failure
// signature collapses these distinct root causes. The auto-learning
// pipeline previously couldn't act on any of them — the failure
// detail names no resource (only a count), so ExtractDescriptivePitfall
// returned nil and the run terminated `stuck` after 2 iterations.
//
// This extractor needs the live `/mock/state` snapshot (not just
// stderr) to identify the lingering resources and look them up in
// the sub-shape table. Caller pattern (run_command.go):
//
//	if failure.Check == "orphan_check" {
//	    state, _ := runtime.Deps.MockState.State(ctx)
//	    routing := generator.ClassifyOrphans(state, sc.Cloud, sc.Name)
//	    for _, p := range routing.Pitfalls {
//	        generator.AppendPitfall(pitfallsDir, cloud, p)
//	    }
//	    for _, g := range routing.MockGaps {
//	        generator.AppendMockGap(docsDir, g)
//	    }
//	    continue
//	}

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// OrphanSubshape classifies the root cause of a lingering resource.
type OrphanSubshape int

const (
	SubshapeUnknown OrphanSubshape = iota
	// SubshapeLLMSoftDelete — provider's destroy returned 200 but the
	// underlying mock keeps the row because the resource has soft-
	// delete semantics (recovery window, scheduled deletion, etc.).
	// LLM-fixable: declare the relevant force/zero-window argument.
	SubshapeLLMSoftDelete
	// SubshapeMockAutoSeed — mock lazy-creates catalogue entries the
	// tenant never owned (e.g. fakeaws's SeedManagedPolicy for
	// arn:aws:iam::aws:policy/*). Mock-side fix: filter from
	// /mock/state. T-B this session was an example.
	SubshapeMockAutoSeed
	// SubshapeMockCascade — parent deleted, child rows orphaned because
	// the mock's schema lacks ON DELETE CASCADE. Mock-side fix.
	SubshapeMockCascade
	// SubshapeMockProviderDivergence — mock Create succeeds, provider
	// Read returns null/partial → tfstate untracked → destroy can't
	// find. Mock-side fix (matching T-D-2 / T-E pattern).
	SubshapeMockProviderDivergence
	// SubshapeProviderSoftDelete — distinct from #1: provider reports
	// destroy success but underlying API only schedules deletion. Mock
	// can sometimes hard-delete to match.
	SubshapeProviderSoftDelete
)

// orphanSubshapeEntry maps a (cloud, service, collection) triple to
// a sub-shape classification + the appropriate prescriptive content.
//
// Seeded with the resources known to have orphan_check incidents
// across the 2026-05-31 + 2026-06-01 sweeps. Extend per scenario as
// new sub-shapes surface.
type orphanSubshapeEntry struct {
	Cloud      string
	Service    string
	Collection string
	Subshape   OrphanSubshape
	// PitfallRule is the prescriptive rule emitted for sub-shape #1.
	// Empty for non-LLM sub-shapes (MockGap channel takes over).
	PitfallRule string
	// MockGapSignal is the signal string written to docs/mock-gaps.md
	// for non-LLM sub-shapes. Empty for sub-shape #1.
	MockGapSignal string
	// Resource is the HCL resource type the pitfall/gap targets (e.g.
	// "aws_secretsmanager_secret"). Required for both channels.
	Resource string
}

var orphanSubshapeTable = []orphanSubshapeEntry{
	// AWS Secrets Manager — default recovery window is 30 days. Provider's
	// DeleteSecret returns 200, mock state lingers in PendingDeletion.
	// Surfaced 2026-05-31 aws-full-stack iter 1.
	{
		Cloud:       "aws",
		Service:     "secretsmanager",
		Collection:  "secrets",
		Subshape:    SubshapeLLMSoftDelete,
		Resource:    "aws_secretsmanager_secret",
		PitfallRule: "`aws_secretsmanager_secret` has soft-delete semantics — AWS schedules deletion with a recovery window (default 30 days). For hermetic test scenarios, set `recovery_window_in_days = 0` on every `aws_secretsmanager_secret` so destroy fully removes the row from mock state (otherwise the orphan_check post-destroy gate fails). Real-cloud scenarios may choose a longer window per their recovery policy.",
	},
	// AWS IAM managed-policy auto-seed (T-B closeout). With the filter
	// in place these don't show up in /mock/state, but if the filter
	// regresses they would — classify as mock-side auto-seed.
	{
		Cloud:         "aws",
		Service:       "iam",
		Collection:    "policies",
		Subshape:      SubshapeMockAutoSeed,
		Resource:      "aws_iam_policy",
		MockGapSignal: "fakeaws auto-seeded managed-policy ARN leaked into /mock/state — filter regression?",
	},
	// AWS subnets — MapPublicIpOnLaunch attribute doesn't persist
	// (T8 in the original backlog, still open). Mock-side fix.
	{
		Cloud:         "aws",
		Service:       "ec2",
		Collection:    "subnets",
		Subshape:      SubshapeMockProviderDivergence,
		Resource:      "aws_subnet",
		MockGapSignal: "fakeaws subnet MapPublicIpOnLaunch attribute doesn't persist (T8); provider wait-loop times out and the row lingers after destroy",
	},
	// AWS KMS keys — destroy schedules deletion of every key in tofu's
	// state, leaving it PendingDeletion (an end state, not an orphan). A
	// key in any other state was never in state at destroy: the provider
	// lost it after fakeaws created it. No aws_kms_key argument fixes
	// that, so it is a mock gap, not a pitfall.
	{
		Cloud:         "aws",
		Service:       "kms",
		Collection:    "keys",
		Subshape:      SubshapeMockProviderDivergence,
		Resource:      "aws_kms_key",
		MockGapSignal: "fakeaws KMS key not PendingDeletion after destroy: destroy never scheduled its deletion, so tofu state had lost the key fakeaws created — a read the provider makes after CreateKey disagrees with fakeaws",
	},
	// GCP Secret Manager — same soft-delete pattern as AWS but with a
	// different argument. google_secret_manager_secret destroy
	// doesn't actually delete; the row stays in PendingDeletion or
	// has time-to-live. Add `deletion_protection = false` and
	// `version_destroy_ttl = 0` (or similar) — exact mechanism varies
	// by provider version, default to "tell the LLM the soft-delete
	// risk and let it pick the right flag".
	{
		Cloud:       "gcp",
		Service:     "secretmanager",
		Collection:  "secrets",
		Subshape:    SubshapeLLMSoftDelete,
		Resource:    "google_secret_manager_secret",
		PitfallRule: "`google_secret_manager_secret` may have soft-delete semantics depending on the provider version (deletion_protection, version_destroy_ttl). For hermetic test scenarios, set whichever argument the provider exposes to disable scheduled deletion so destroy fully removes the row (check the provider docs for the canonical name). The orphan_check post-destroy gate flags PendingDeletion rows.",
	},
	// GCP KMS crypto keys — google_kms_crypto_key always has a 24h-
	// to-30d destroy schedule on rotation. force-destroy isn't
	// available; the mock can hard-delete on its side to match
	// "infra is fully gone" semantics. Mixed: mock-side fix preferred.
	{
		Cloud:         "gcp",
		Service:       "kms",
		Collection:    "crypto_keys",
		Subshape:      SubshapeProviderSoftDelete,
		Resource:      "google_kms_crypto_key",
		MockGapSignal: "google_kms_crypto_key destroy schedules deletion (no force flag) — fakegcp should hard-delete on its side to match the 'infra fully gone' semantic the orphan_check gate expects",
	},
}

// OrphanedResource is a single lingering entry observed in
// /mock/state after destroy.
type OrphanedResource struct {
	Service    string // e.g. "secretsmanager", "kms", "iam"
	Collection string // e.g. "secrets", "keys", "policies"
	Index      int    // position in the array (for diagnostics)
}

// OrphanRouting is the result of classifying orphans. The two
// channels are populated independently (one orphan never appears in
// both); the caller fans them out to AppendPitfall + AppendMockGap.
type OrphanRouting struct {
	Pitfalls []LearnedPitfall
	MockGaps []MockGap
	// Unclassified lists orphans the sub-shape table didn't cover.
	// Caller can log these for human triage / future table seeding.
	Unclassified []OrphanedResource
}

// ignoredOrphanRoots and ignoredOrphanCollections are mock bookkeeping, not
// tenant resources, so they never count as orphans on a clean teardown.
var (
	ignoredOrphanRoots = map[string]bool{
		"metadata": true,
		// fakegcp persists every long-running operation (insert, delete,
		// setLabels, ...) and surfaces them under state.operations. They
		// are the audit trail of the API calls that made the resources,
		// not resources.
		"operations": true,
		// fakeaws's universal bookkeeping tables (fakeaws concepts.md
		// "Required surface" item 7): `audit` is the per-request log and
		// `schema_version` the integer schema marker /mock/state emits.
		"audit":          true,
		"schema_version": true,
	}
	ignoredOrphanCollections = map[string]bool{
		"events":   true,
		"metrics":  true,
		"messages": true,
	}
)

// mockwayDefaultProjectID is the project mockway seeds at boot (S140) so a
// scenario that never declares one still has a valid project_id to send.
// That row is present on every clean teardown, so counting it would fail
// the orphan check for every scenario in the suite. Only the seeded
// default is exempt: a project a scenario created and failed to destroy
// still counts, which matters now that Layer 3 stacks create their own
// projects. It mirrors models.DefaultProjectID in mockway; infrafactory
// does not import the sibling mocks.
const mockwayDefaultProjectID = "00000000-0000-0000-0000-000000000000"

// autoCreatedVPCName is the name of the default VPC Scaleway gives a
// project, where it puts a private network whose request names no vpc_id;
// mockway models that (mockway#26). Like the "Default security group"
// ADR-0023's purge exists for, Terraform never creates or destroys it, so
// counting it would fail the orphan check for every scenario with private
// networking: every scenario with compute, because vpc_required demands
// it. Matched on the name the API assigns, not on absence from state, so a
// VPC a scenario declared and failed to destroy still counts. A stack that
// names its own VPC "default" is exempted wrongly, the known edge of this
// rule and cheaper than trusting state to be complete.
const autoCreatedVPCName = "default"

// kmsPendingDeletion is the state AWS leaves a destroyed KMS key in: it
// cannot be deleted sooner than its waiting period, so a clean destroy
// ends there.
const kmsPendingDeletion = "PendingDeletion"

// leftByCleanDestroy reports whether item, a row of root.collection in a
// /mock/state snapshot, is one a clean destroy leaves behind: a fixture
// the mock creates itself, or a resource's end state. Only these exact
// rows are exempt; a project, VPC or key the stack made and failed to
// destroy is an orphan.
func leftByCleanDestroy(root, collection string, item map[string]any) bool {
	switch {
	case root == "account" && collection == "projects":
		id, _ := item["id"].(string)
		return id == mockwayDefaultProjectID
	case root == "vpc" && collection == "vpcs":
		name, _ := item["name"].(string)
		return name == autoCreatedVPCName
	case root == "kms" && collection == "keys":
		state, _ := item["state"].(string)
		return state == kmsPendingDeletion
	}
	return false
}

// OrphanCollection is a collection in a /mock/state snapshot and the rows
// in it a clean destroy should have removed.
type OrphanCollection struct {
	Service    string
	Collection string
	// Indexes are the orphans' positions in the collection's /mock/state
	// array, exempt rows included in the numbering.
	Indexes []int
}

// Orphans decodes a /mock/state snapshot ({ service: { collection: [...] } })
// and returns each collection still holding an orphan, sorted by service
// then collection. It is the one definition of an orphan: the post-destroy
// gate counts what it returns and ClassifyOrphans routes it. A row that is
// not an object counts, since an unreadable row is no evidence of a clean
// destroy, and a snapshot that is not an object is an error.
func Orphans(stateJSON []byte) ([]OrphanCollection, error) {
	var state map[string]any
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, fmt.Errorf("decode state snapshot: %w", err)
	}
	if state == nil {
		return nil, errors.New("decode state snapshot: not an object")
	}
	var orphans []OrphanCollection
	for _, service := range slices.Sorted(maps.Keys(state)) {
		collections, ok := state[service].(map[string]any)
		if !ok || ignoredOrphanRoots[service] {
			continue
		}
		for _, collection := range slices.Sorted(maps.Keys(collections)) {
			items, ok := collections[collection].([]any)
			if !ok || ignoredOrphanCollections[collection] {
				continue
			}
			var indexes []int
			for i, item := range items {
				if m, ok := item.(map[string]any); ok && leftByCleanDestroy(service, collection, m) {
					continue
				}
				indexes = append(indexes, i)
			}
			if len(indexes) > 0 {
				orphans = append(orphans, OrphanCollection{Service: service, Collection: collection, Indexes: indexes})
			}
		}
	}
	return orphans, nil
}

// ClassifyOrphans walks the /mock/state JSON and classifies each
// non-empty resource collection per the orphan sub-shape table.
//
// mockStateJSON is the raw bytes from GET /mock/state (the same
// shape every mock returns: { service: { collection: [...] } }).
//
// cloud is the scenario's cloud ("aws" | "gcp" | "scaleway").
// Lookups are scoped by cloud so the same (service, collection)
// pair across clouds doesn't false-match.
//
// scenario is propagated into the emitted entries' DiscoveredFrom
// (pitfall) and Scenario (mock-gap) fields.
//
// timestamp is propagated into the emitted MockGap.Timestamp field.
// Caller typically passes the run ID.
//
// What counts as an orphan is Orphans', so the classifier routes exactly
// what the post-destroy gate counted. Best-effort: a snapshot Orphans
// cannot decode returns an empty routing rather than an error (the
// orphan_check gate already failed; classification is sugar). Unknown
// collections are recorded in routing.Unclassified for human triage.
func ClassifyOrphans(mockStateJSON []byte, cloud, scenario, timestamp string) OrphanRouting {
	var routing OrphanRouting
	if len(mockStateJSON) == 0 || cloud == "" {
		return routing
	}

	orphans, err := Orphans(mockStateJSON)
	if err != nil {
		return routing
	}
	for _, orphan := range orphans {
		service, collection := orphan.Service, orphan.Collection
		// Look up in the sub-shape table.
		entry := lookupOrphanSubshape(cloud, service, collection)
		if entry == nil {
			for _, i := range orphan.Indexes {
				routing.Unclassified = append(routing.Unclassified, OrphanedResource{
					Service:    service,
					Collection: collection,
					Index:      i,
				})
			}
			continue
		}
		// Emit one entry per orphan (some scenarios leave multiple
		// instances of the same resource type — count > 1).
		switch entry.Subshape {
		case SubshapeLLMSoftDelete:
			// Single pitfall per resource type; AppendPitfall
			// dedups across multiple invocations.
			routing.Pitfalls = append(routing.Pitfalls, LearnedPitfall{
				Resource:       entry.Resource,
				Rule:           entry.PitfallRule,
				DiscoveredFrom: scenario,
			})
		case SubshapeMockAutoSeed,
			SubshapeMockCascade,
			SubshapeMockProviderDivergence,
			SubshapeProviderSoftDelete:
			routing.MockGaps = append(routing.MockGaps, MockGap{
				Cloud:    cloud,
				Signal:   entry.MockGapSignal,
				Resource: entry.Resource,
				Scenario: scenario,
				Detail: fmt.Sprintf("orphan_check: %s.%s had %d non-empty entries after destroy",
					service, collection, len(orphan.Indexes)),
				Timestamp: timestamp,
			})
		}
	}
	return routing
}

func lookupOrphanSubshape(cloud, service, collection string) *orphanSubshapeEntry {
	for i := range orphanSubshapeTable {
		entry := &orphanSubshapeTable[i]
		if entry.Cloud == cloud && entry.Service == service && entry.Collection == collection {
			return entry
		}
	}
	return nil
}
