package generator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// awsSubnetAvoidRule is pitfalls/aws.yaml:8 verbatim.
const awsSubnetAvoidRule = "exit status 1 | stderr: ╷ Do NOT use attribute `map_public_ip_on_launch` on `aws_subnet` — observed in scenario \"aws-eks\" to cause the failure above."

const subnetShape = `resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "a" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.0.1.0/24"
  map_public_ip_on_launch = true
}
`

func intp(v int) *int { return &v }

func writeAWSCorpus(t *testing.T, dir string, entries ...PitfallEntry) {
	t.Helper()
	require.NoError(t, writePitfallsFile(dir, filepath.Join(dir, "aws.yaml"), "aws", &PitfallsFile{Provider: "aws", Pitfalls: entries}))
}

func subnetAvoidEntry() PitfallEntry {
	return PitfallEntry{Resource: "aws_subnet", Rule: awsSubnetAvoidRule, Source: AvoidSource, DiscoveredFrom: "aws-eks", LearnedLayer: MockDeployLayer}
}

func unrelatedEntry() PitfallEntry {
	return PitfallEntry{Resource: "aws_s3_bucket", Rule: "bucket names must be lowercase", Source: "descriptive", DiscoveredFrom: "aws-full-stack"}
}

// storeShape writes files under the shape dir for id and returns its hash.
func storeShape(t *testing.T, pitfallsDir, id string, files map[string]string) string {
	t.Helper()
	dir := avoidShapeDir(pitfallsDir, id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	sum, err := ShapeSHA256(dir)
	require.NoError(t, err)
	return sum
}

func contradictedCheck(id, sha string) AvoidCheck {
	return AvoidCheck{ID: id, At: "2026-09-28T12:00:00Z", From: "testdata/web-step-one", ShapeSHA256: sha, ApplyExit: intp(0), PlanExit: intp(0), Outcome: AvoidOutcomeContradicted}
}

func subnetRetirement(check AvoidCheck) AvoidRetirement {
	return AvoidRetirement{Resource: "aws_subnet", Attributes: []string{"map_public_ip_on_launch"}, Check: check}
}

// snapshotFiles returns the corpus and ledger bytes; a missing file is nil.
func snapshotFiles(t *testing.T, pitfallsDir string) [2][]byte {
	t.Helper()
	var out [2][]byte
	for i, p := range []string{filepath.Join(pitfallsDir, "aws.yaml"), filepath.Join(avoidChecksDir(pitfallsDir), "aws.yaml")} {
		b, err := os.ReadFile(p)
		if !os.IsNotExist(err) {
			require.NoError(t, err)
		}
		out[i] = b
	}
	return out
}

func readLedger(t *testing.T, pitfallsDir string) *AvoidLedger {
	t.Helper()
	l, err := ReadAvoidLedger(pitfallsDir, "aws")
	require.NoError(t, err)
	return l
}

func TestShapeSHA256(t *testing.T) {
	write := func(t *testing.T, names []string, files map[string]string) string {
		dir := t.TempDir()
		for _, n := range names {
			require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte(files[n]), 0o644))
		}
		return dir
	}
	files := map[string]string{"main.tf": subnetShape, "vpc.tf": `resource "aws_vpc" "b" {}`}

	a, err := ShapeSHA256(write(t, []string{"main.tf", "vpc.tf"}, files))
	require.NoError(t, err)
	b, err := ShapeSHA256(write(t, []string{"vpc.tf", "main.tf"}, files))
	require.NoError(t, err)
	assert.Equal(t, a, b, "write order must not change the hash")

	renamed := write(t, []string{"main.tf"}, files)
	require.NoError(t, os.WriteFile(filepath.Join(renamed, "other.tf"), []byte(files["vpc.tf"]), 0o644))
	c, err := ShapeSHA256(renamed)
	require.NoError(t, err)
	assert.NotEqual(t, a, c, "a renamed file changes the hash")

	edited := write(t, []string{"main.tf", "vpc.tf"}, files)
	require.NoError(t, os.WriteFile(filepath.Join(edited, "vpc.tf"), []byte(files["vpc.tf"]+"\n"), 0o644))
	d, err := ShapeSHA256(edited)
	require.NoError(t, err)
	assert.NotEqual(t, a, d, "an edited file changes the hash")

	withSubdir := write(t, []string{"main.tf"}, files)
	require.NoError(t, os.Mkdir(filepath.Join(withSubdir, "nested"), 0o755))
	_, err = ShapeSHA256(withSubdir)
	assert.Error(t, err, "a subdirectory is an error")

	withReadme := write(t, []string{"main.tf"}, files)
	require.NoError(t, os.WriteFile(filepath.Join(withReadme, "README.md"), []byte("x"), 0o644))
	_, err = ShapeSHA256(withReadme)
	assert.Error(t, err, "a non-.tf file is an error")

	_, err = ShapeSHA256(t.TempDir())
	assert.Error(t, err, "an empty shape is an error")
}

func TestParseAvoidRule(t *testing.T) {
	for _, attrs := range [][]string{{"map_public_ip_on_launch"}, {"map_public_ip_on_launch", "assign_ipv6_address_on_creation"}} {
		rule := buildAvoidRule("aws_subnet", attrs, nil, "Error: waiting for EC2 Subnet. More.", "aws-eks")
		resource, got, ok := ParseAvoidRule(rule)
		require.True(t, ok, rule)
		assert.Equal(t, "aws_subnet", resource)
		assert.Equal(t, attrs, got)
	}

	resource, attrs, ok := ParseAvoidRule(awsSubnetAvoidRule)
	require.True(t, ok)
	assert.Equal(t, "aws_subnet", resource)
	assert.Equal(t, []string{"map_public_ip_on_launch"}, attrs)

	for name, rule := range map[string]string{
		"gcp resource type": "exit status 1 | stderr: ╷ Do NOT use resource type `google_compute_instance_template` on `google_compute_instance_template` — observed in scenario \"gcp-load-balancer\" to cause the failure above.",
		"mixed":             buildAvoidRule("aws_subnet", []string{"map_public_ip_on_launch"}, []string{"aws_eip"}, "Error.", "aws-eks"),
		"free text":         "Do not set map_public_ip_on_launch on aws_subnet; it times out on the mock.",
	} {
		_, _, ok := ParseAvoidRule(rule)
		assert.False(t, ok, name)
	}
}

func TestRetireAvoidPitfall_Retires(t *testing.T) {
	dir := t.TempDir()
	writeAWSCorpus(t, dir, unrelatedEntry(), subnetAvoidEntry())
	sha := storeShape(t, dir, "chk-1", map[string]string{"main.tf": subnetShape})

	rec, err := RetireAvoidPitfall(dir, "aws", subnetRetirement(contradictedCheck("chk-1", sha)))
	require.NoError(t, err)
	assert.Equal(t, AvoidRecordRetired, rec.Status)

	entries, err := LoadPitfallEntries(dir, "aws")
	require.NoError(t, err)
	assert.Equal(t, []PitfallEntry{unrelatedEntry()}, entries)

	ledger := readLedger(t, dir)
	require.Len(t, ledger.Records, 1)
	got := ledger.Records[0]
	assert.Equal(t, subnetAvoidEntry(), *got.Entry, "the whole entry is kept")
	assert.Equal(t, MockDeployLayer, got.LearnedLayer)
	assert.Equal(t, "chk-1", got.Check.ID)
}

func TestRetireAvoidPitfall_Refusals(t *testing.T) {
	sandbox := subnetAvoidEntry()
	sandbox.LearnedLayer = "sandbox_deploy"
	legacy := subnetAvoidEntry()
	legacy.LearnedLayer = ""
	freeText := subnetAvoidEntry()
	freeText.Rule = "Do not set map_public_ip_on_launch on aws_subnet."
	camelDescriptive := PitfallEntry{Resource: "aws_subnet", Rule: "waiting for MapPublicIpOnLaunch update: timeout", Source: "descriptive"}

	type fixture struct {
		entries []PitfallEntry
		shape   string
		ledger  string // raw ledger bytes, written before the call
		edit    func(*AvoidRetirement)
		want    string
	}
	cases := map[string]fixture{
		"rule does not parse":            {want: "not an avoid rule", entries: []PitfallEntry{freeText}},
		"outcome not contradicted":       {want: "not recurred or inconclusive", edit: func(r *AvoidRetirement) { r.Check.Outcome = "bogus" }},
		"contradicted with apply exit 1": {want: "needs apply_exit 0", edit: func(r *AvoidRetirement) { r.Check.ApplyExit = intp(1) }},
		"contradicted with plan exit 2":  {want: "needs apply_exit 0", edit: func(r *AvoidRetirement) { r.Check.PlanExit = intp(2) }},
		"empty check id":                 {want: "are required", edit: func(r *AvoidRetirement) { r.Check.ID = "" }},
		"empty check at":                 {want: "not RFC3339", edit: func(r *AvoidRetirement) { r.Check.At = "" }},
		"empty check from":               {want: "are required", edit: func(r *AvoidRetirement) { r.Check.From = "" }},
		"empty shape sha":                {want: "are required", edit: func(r *AvoidRetirement) { r.Check.ShapeSHA256 = "" }},
		"missing apply exit":             {want: "are required", edit: func(r *AvoidRetirement) { r.Check.ApplyExit = nil }},
		"missing plan exit":              {want: "are required", edit: func(r *AvoidRetirement) { r.Check.PlanExit = nil }},
		"shape sha differs":              {want: "hashes to", edit: func(r *AvoidRetirement) { r.Check.ShapeSHA256 = "0000" }},
		"shape sets the attribute false": {want: "sets every attribute", shape: `resource "aws_subnet" "a" { map_public_ip_on_launch = false }`},
		"legacy layer without evidence":  {want: "no layer_evidence", entries: []PitfallEntry{legacy}},
		"sandbox-learned entry":          {want: "learned at sandbox_deploy", entries: []PitfallEntry{sandbox}, edit: func(r *AvoidRetirement) { r.LayerEvidence = "run x" }},
		"a descriptive entry names it":   {want: "source descriptive", entries: []PitfallEntry{subnetAvoidEntry(), camelDescriptive}},
		"no entry names it":              {want: "no corpus entry", entries: []PitfallEntry{unrelatedEntry()}},
		"malformed ledger":               {want: "ledger unreadable", ledger: "provider: aws\nrecords: [not, a, record\n"},
		"ledger with an unknown field":   {want: "ledger unreadable", ledger: "provider: aws\nrecords: []\nextra: 1\n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.entries == nil {
				tc.entries = []PitfallEntry{subnetAvoidEntry()}
			}
			writeAWSCorpus(t, dir, tc.entries...)
			if tc.shape == "" {
				tc.shape = subnetShape
			}
			sha := storeShape(t, dir, "chk-1", map[string]string{"main.tf": tc.shape})
			if tc.ledger != "" {
				require.NoError(t, os.WriteFile(filepath.Join(avoidChecksDir(dir), "aws.yaml"), []byte(tc.ledger), 0o644))
			}
			req := subnetRetirement(contradictedCheck("chk-1", sha))
			if tc.edit != nil {
				tc.edit(&req)
			}
			before := snapshotFiles(t, dir)

			_, err := RetireAvoidPitfall(dir, "aws", req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "refusing")
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, before, snapshotFiles(t, dir), "corpus and ledger must be byte-identical")
		})
	}
}

func TestRetireAvoidPitfall_LegacyLayerWithEvidence(t *testing.T) {
	dir := t.TempDir()
	legacy := subnetAvoidEntry()
	legacy.LearnedLayer = ""
	writeAWSCorpus(t, dir, legacy)
	sha := storeShape(t, dir, "chk-1", map[string]string{"main.tf": subnetShape})
	req := subnetRetirement(contradictedCheck("chk-1", sha))
	req.LayerEvidence = "run 20260603T214517Z iteration 1"

	rec, err := RetireAvoidPitfall(dir, "aws", req)
	require.NoError(t, err)
	assert.Equal(t, MockDeployLayer, rec.LearnedLayer)
	assert.Equal(t, "run 20260603T214517Z iteration 1", rec.LayerEvidence)
	assert.Empty(t, rec.Entry.LearnedLayer, "the entry is recorded as it was")
}

func TestRetireAvoidPitfall_MultiAttribute(t *testing.T) {
	attrs := []string{"map_public_ip_on_launch", "assign_ipv6_address_on_creation"}
	entry := PitfallEntry{Resource: "aws_subnet", Rule: buildAvoidRule("aws_subnet", attrs, nil, "Error.", "aws-eks"), Source: AvoidSource, LearnedLayer: MockDeployLayer}
	both := `resource "aws_subnet" "a" {
  map_public_ip_on_launch         = true
  assign_ipv6_address_on_creation = true
}
`
	req := func(sha string) AvoidRetirement {
		return AvoidRetirement{Resource: "aws_subnet", Attributes: attrs, Check: contradictedCheck("chk-1", sha)}
	}

	t.Run("retires whole", func(t *testing.T) {
		dir := t.TempDir()
		writeAWSCorpus(t, dir, entry)
		rec, err := RetireAvoidPitfall(dir, "aws", req(storeShape(t, dir, "chk-1", map[string]string{"main.tf": both})))
		require.NoError(t, err)
		assert.Equal(t, attrs, rec.Attributes)
		entries, err := LoadPitfallEntries(dir, "aws")
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("shape setting one is refused", func(t *testing.T) {
		dir := t.TempDir()
		writeAWSCorpus(t, dir, entry)
		sha := storeShape(t, dir, "chk-1", map[string]string{"main.tf": subnetShape})
		before := snapshotFiles(t, dir)
		_, err := RetireAvoidPitfall(dir, "aws", req(sha))
		require.Error(t, err)
		assert.Equal(t, before, snapshotFiles(t, dir))
	})

	t.Run("a check naming one is refused", func(t *testing.T) {
		dir := t.TempDir()
		writeAWSCorpus(t, dir, entry)
		sha := storeShape(t, dir, "chk-1", map[string]string{"main.tf": both})
		before := snapshotFiles(t, dir)
		_, err := RetireAvoidPitfall(dir, "aws", subnetRetirement(contradictedCheck("chk-1", sha)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "every attribute")
		assert.Equal(t, before, snapshotFiles(t, dir))
	})
}

func TestRetireAvoidPitfall_KeptAndAppendOnly(t *testing.T) {
	dir := t.TempDir()
	other := PitfallEntry{Resource: "aws_instance", Rule: buildAvoidRule("aws_instance", []string{"ipv6_address_count"}, nil, "Error.", "aws-web"), Source: AvoidSource, LearnedLayer: MockDeployLayer}
	writeAWSCorpus(t, dir, unrelatedEntry(), subnetAvoidEntry(), other)
	corpusBefore, err := LoadPitfallEntries(dir, "aws")
	require.NoError(t, err)

	// Kept: the corpus is untouched and a kept record is appended.
	sha := storeShape(t, dir, "chk-kept", map[string]string{"main.tf": subnetShape})
	kept := contradictedCheck("chk-kept", sha)
	kept.Outcome, kept.ApplyExit, kept.Detail = AvoidOutcomeRecurred, intp(1), "MapPublicIpOnLaunch update: timeout"
	before := snapshotFiles(t, dir)
	rec, err := RetireAvoidPitfall(dir, "aws", subnetRetirement(kept))
	require.NoError(t, err)
	assert.Equal(t, AvoidRecordKept, rec.Status)
	assert.Equal(t, before[0], snapshotFiles(t, dir)[0], "a kept outcome leaves the corpus byte-identical")

	// Two retirements: earlier records stay byte-identical.
	sha = storeShape(t, dir, "chk-1", map[string]string{"main.tf": subnetShape})
	_, err = RetireAvoidPitfall(dir, "aws", subnetRetirement(contradictedCheck("chk-1", sha)))
	require.NoError(t, err)
	firstTwo := snapshotFiles(t, dir)[1]

	sha = storeShape(t, dir, "chk-2", map[string]string{"main.tf": `resource "aws_instance" "web" { ipv6_address_count = 1 }`})
	_, err = RetireAvoidPitfall(dir, "aws", AvoidRetirement{Resource: "aws_instance", Attributes: []string{"ipv6_address_count"}, Check: contradictedCheck("chk-2", sha)})
	require.NoError(t, err)
	all := snapshotFiles(t, dir)[1]
	assert.True(t, bytes.HasPrefix(all, firstTwo), "earlier records are byte-identical:\n%s\n---\n%s", firstTwo, all)
	require.Len(t, readLedger(t, dir).Records, 3)

	corpusAfter, err := LoadPitfallEntries(dir, "aws")
	require.NoError(t, err)
	retired := 0
	for _, r := range readLedger(t, dir).Records {
		if r.Status == AvoidRecordRetired {
			retired++
		}
	}
	assert.Equal(t, len(corpusBefore), len(corpusAfter)+retired)
	assert.Equal(t, []PitfallEntry{unrelatedEntry()}, corpusAfter)
}
