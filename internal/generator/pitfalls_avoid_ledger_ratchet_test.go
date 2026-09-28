package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetiredAvoidPitfallsStayRetired: a retired avoid rule cannot come
// back through any path that bypasses AppendPitfall -- a hand edit, the
// API's PUT, pitfall-merge -- and every retirement keeps its evidence.
// The ledger is pitfalls/avoid-checks/<cloud>.yaml (ADR-0034, amendment 2026-09-28).
func TestRetiredAvoidPitfallsStayRetired(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "pitfalls")
	corpora, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, corpora)
	for _, path := range corpora {
		cloud := strings.TrimSuffix(filepath.Base(path), ".yaml")
		t.Run(cloud, func(t *testing.T) {
			assert.Empty(t, retiredAvoidViolations(dir, cloud))
		})
	}
}

// retiredAvoidViolations reports a malformed ledger, a stored shape that
// no longer hashes to its record, and any non-live corpus entry naming an
// attribute the ledger retired on its resource and no later non-mock
// relearned record re-admitted.
func retiredAvoidViolations(pitfallsDir, cloud string) []string {
	ledger, err := ReadAvoidLedger(pitfallsDir, cloud)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	for i, r := range ledger.Records {
		if r.Check == nil {
			continue
		}
		sum, err := ShapeSHA256(avoidShapeDir(pitfallsDir, r.Check.ID))
		if err != nil {
			out = append(out, fmt.Sprintf("record %d: %v", i, err))
		} else if sum != r.Check.ShapeSHA256 {
			out = append(out, fmt.Sprintf("record %d: shape %s hashes to %s, recorded %s", i, r.Check.ID, sum, r.Check.ShapeSHA256))
		}
	}
	entries, err := LoadPitfallEntries(pitfallsDir, cloud)
	if err != nil {
		return append(out, err.Error())
	}
	for i, e := range entries {
		if e.Source == LiveSource {
			continue
		}
		for _, h := range ledger.retiredAttributesNamed(e.Resource, e.Rule) {
			out = append(out, fmt.Sprintf("%s.yaml entry %d (%s on %s) names `%s`, retired by check %s", cloud, i, e.Source, e.Resource, h.Attribute, h.CheckID))
		}
	}
	return out
}

func TestRetiredAvoidViolations(t *testing.T) {
	// writeRawLedger bypasses validation, as a hand edit would.
	writeRawLedger := func(t *testing.T, dir string, edit func(*AvoidLedger)) {
		t.Helper()
		ledger := readLedger(t, dir)
		edit(ledger)
		ledgerDir := avoidChecksDir(dir)
		require.NoError(t, writePitfallsFile(ledgerDir, filepath.Join(ledgerDir, "aws.yaml"), "aws", ledger))
	}
	relearned := func(layer string) AvoidLedgerRecord {
		return AvoidLedgerRecord{Status: AvoidRecordRelearned, Resource: "aws_subnet", Attributes: []string{"map_public_ip_on_launch"}, LearnedLayer: layer, Rule: "r", At: "2026-09-29T00:00:00Z"}
	}
	reentered := func(source, rule string) func(*testing.T, string) {
		return func(t *testing.T, dir string) {
			writeAWSCorpus(t, dir, unrelatedEntry(), PitfallEntry{Resource: "aws_subnet", Rule: rule, Source: source, LearnedLayer: MockDeployLayer})
		}
	}

	fails := map[string]func(*testing.T, string){
		"avoid entry re-entered":    reentered(AvoidSource, awsSubnetAvoidRule),
		"fix entry names it":        reentered(FixSource, "Set `map_public_ip_on_launch = false` explicitly."),
		"descriptive camel case":    reentered("descriptive", "waiting for EC2 Subnet MapPublicIpOnLaunch update: timeout"),
		"unparseable avoid wording": reentered(AvoidSource, "Never set map_public_ip_on_launch."),
		"relearned before retirement": func(t *testing.T, dir string) {
			writeRawLedger(t, dir, func(l *AvoidLedger) {
				l.Records = append([]AvoidLedgerRecord{relearned("sandbox_deploy")}, l.Records...)
			})
			reentered(AvoidSource, awsSubnetAvoidRule)(t, dir)
		},
		"retired record missing from": func(t *testing.T, dir string) {
			writeRawLedger(t, dir, func(l *AvoidLedger) { l.Records[0].Check.From = "" })
		},
		"retired record missing layer": func(t *testing.T, dir string) {
			writeRawLedger(t, dir, func(l *AvoidLedger) { l.Records[0].LearnedLayer = "" })
		},
		"non-mock entry without layer_evidence": func(t *testing.T, dir string) {
			writeRawLedger(t, dir, func(l *AvoidLedger) { l.Records[0].Entry.LearnedLayer = "" })
		},
		"retired at a non-mock layer": func(t *testing.T, dir string) {
			writeRawLedger(t, dir, func(l *AvoidLedger) {
				l.Records[0].LearnedLayer = "sandbox_deploy"
				l.Records[0].LayerEvidence = "run x"
			})
		},
		"shape sha mismatch": func(t *testing.T, dir string) {
			shape := filepath.Join(avoidShapeDir(dir, "chk-1"), "main.tf")
			require.NoError(t, os.WriteFile(shape, []byte(subnetShape+"# edited\n"), 0o644))
		},
		"malformed ledger": func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(avoidChecksDir(dir), "aws.yaml"), []byte("records: [\n"), 0o644))
		},
	}
	for name, mutate := range fails {
		t.Run("fails/"+name, func(t *testing.T) {
			dir := retiredLedgerFixture(t)
			mutate(t, dir)
			assert.NotEmpty(t, retiredAvoidViolations(dir, "aws"))
		})
	}

	passes := map[string]func(*testing.T, string){
		"as retired": func(*testing.T, string) {},
		"live entry names it": func(t *testing.T, dir string) {
			writeAWSCorpus(t, dir, PitfallEntry{Resource: "aws_subnet", Rule: "MapPublicIpOnLaunch observed off", Source: LiveSource, LastSeen: "2026-09-28T00:00:00Z"})
		},
		"later non-mock relearned record": func(t *testing.T, dir string) {
			writeRawLedger(t, dir, func(l *AvoidLedger) { l.Records = append(l.Records, relearned("sandbox_deploy")) })
			reentered(AvoidSource, awsSubnetAvoidRule)(t, dir)
		},
		"no ledger": func(t *testing.T, dir string) {
			require.NoError(t, os.RemoveAll(avoidChecksDir(dir)))
			reentered(AvoidSource, awsSubnetAvoidRule)(t, dir)
		},
	}
	for name, mutate := range passes {
		t.Run("passes/"+name, func(t *testing.T) {
			dir := retiredLedgerFixture(t)
			mutate(t, dir)
			assert.Empty(t, retiredAvoidViolations(dir, "aws"))
		})
	}
}
