package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

// A `source: avoid` rule leaves the corpus only into the avoid-check
// ledger, pitfalls/avoid-checks/<cloud>.yaml, and only on evidence that
// contradicts it: the shape it forbids applied cleanly against the mock
// (ADR-0034, amendment 2026-09-28). The ledger is append-only and keeps
// the whole entry, so a retirement can be read back and replayed.
//
// Mock evidence retires only a mock-learned rule. A later mock-learned
// recurrence is a mock regression and is refused; a real-cloud one is
// re-learned beside a `relearned` record.

// MockDeployLayer is the Layer 2 layer name: the only layer whose lessons
// the mock can contradict.
const MockDeployLayer = "mock_deploy"

// Ledger record statuses.
const (
	AvoidRecordRetired   = "retired"
	AvoidRecordKept      = "kept"
	AvoidRecordRelearned = "relearned"
)

// Check outcomes. Only contradicted retires; the others keep the rule.
const (
	AvoidOutcomeContradicted = "contradicted"
	AvoidOutcomeRecurred     = "recurred"
	AvoidOutcomeInconclusive = "inconclusive"
)

const avoidChecksDirName = "avoid-checks"

// AvoidCheck is one replay of a rule's forbidden shape against the mock.
// The exits are pointers so a missing one reads as missing, not as 0.
type AvoidCheck struct {
	ID          string `yaml:"id"`
	At          string `yaml:"at"`
	From        string `yaml:"from"`
	ShapeSHA256 string `yaml:"shape_sha256"`
	ApplyExit   *int   `yaml:"apply_exit"`
	PlanExit    *int   `yaml:"plan_exit"`
	Outcome     string `yaml:"outcome"`
	Detail      string `yaml:"detail,omitempty"`
}

// AvoidLedgerRecord is one ledger line. Retired and kept records carry
// the entry and the check; relearned records carry the rule that
// re-admitted the attributes, and when.
type AvoidLedgerRecord struct {
	Status        string        `yaml:"status"`
	Resource      string        `yaml:"resource"`
	Attributes    []string      `yaml:"attributes"`
	LearnedLayer  string        `yaml:"learned_layer"`
	LayerEvidence string        `yaml:"layer_evidence,omitempty"`
	Entry         *PitfallEntry `yaml:"entry,omitempty"`
	Check         *AvoidCheck   `yaml:"check,omitempty"`

	DiscoveredFrom string `yaml:"discovered_from,omitempty"`
	Rule           string `yaml:"rule,omitempty"`
	At             string `yaml:"at,omitempty"`
}

// AvoidLedger is the whole ledger file for one cloud.
type AvoidLedger struct {
	Provider string              `yaml:"provider"`
	Records  []AvoidLedgerRecord `yaml:"records"`
}

// AvoidRetirement is what a check hands RetireAvoidPitfall. LayerEvidence
// is required when the entry predates learned_layer: it names the run
// artifacts that establish the rule was learned on the mock.
type AvoidRetirement struct {
	Resource      string
	Attributes    []string
	LayerEvidence string
	Check         AvoidCheck
}

func avoidChecksDir(pitfallsDir string) string {
	return filepath.Join(pitfallsDir, avoidChecksDirName)
}

func avoidShapeDir(pitfallsDir, checkID string) string {
	return filepath.Join(avoidChecksDir(pitfallsDir), "shapes", checkID)
}

// ReadAvoidLedger reads and validates the cloud's ledger. A missing
// ledger is an empty one; an unreadable or malformed one is an error.
func ReadAvoidLedger(pitfallsDir, cloud string) (*AvoidLedger, error) {
	if err := assertCloudName(cloud); err != nil {
		return nil, err
	}
	path := filepath.Join(avoidChecksDir(pitfallsDir), cloud+".yaml")
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &AvoidLedger{Provider: cloud}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read avoid-check ledger %s: %w", path, err)
	}

	var ledger AvoidLedger
	dec := yaml.NewDecoder(bytes.NewReader(payload))
	dec.KnownFields(true)
	if err := dec.Decode(&ledger); err != nil {
		if errors.Is(err, io.EOF) {
			err = errors.New("file is empty")
		}
		return nil, fmt.Errorf("parse avoid-check ledger %s: %w", path, err)
	}
	if ledger.Provider != cloud {
		return nil, fmt.Errorf("avoid-check ledger %s: provider %q, want %q", path, ledger.Provider, cloud)
	}
	for i, rec := range ledger.Records {
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("avoid-check ledger %s: record %d: %w", path, i, err)
		}
	}
	return &ledger, nil
}

func (r AvoidLedgerRecord) validate() error {
	if r.Resource == "" || len(r.Attributes) == 0 || slices.Contains(r.Attributes, "") {
		return errors.New("resource and attributes are required")
	}
	if r.LearnedLayer == "" {
		return errors.New("learned_layer is required")
	}
	switch r.Status {
	case AvoidRecordRelearned:
		if r.LearnedLayer == MockDeployLayer {
			return errors.New("a relearned record must come from a layer other than mock_deploy")
		}
		if r.Rule == "" {
			return errors.New("a relearned record needs its rule")
		}
		return validRFC3339("at", r.At)
	case AvoidRecordRetired, AvoidRecordKept:
		return r.validateChecked()
	default:
		return fmt.Errorf("status %q is not retired, kept or relearned", r.Status)
	}
}

func (r AvoidLedgerRecord) validateChecked() error {
	if r.Entry == nil || r.Check == nil {
		return fmt.Errorf("a %s record needs the entry and the check", r.Status)
	}
	if r.LearnedLayer != MockDeployLayer {
		return fmt.Errorf("learned_layer %q: only a mock_deploy rule is checked against the mock", r.LearnedLayer)
	}
	if r.Entry.LearnedLayer != MockDeployLayer && r.LayerEvidence == "" {
		return fmt.Errorf("the entry's learned_layer is %q and no layer_evidence establishes it was mock_deploy", r.Entry.LearnedLayer)
	}
	c := r.Check
	if c.ID == "" || c.From == "" || c.ShapeSHA256 == "" || c.ApplyExit == nil || c.PlanExit == nil {
		return errors.New("check id, from, shape_sha256, apply_exit and plan_exit are required")
	}
	if !checkIDRe.MatchString(c.ID) {
		return fmt.Errorf("check id %q is not a plain name", c.ID)
	}
	if err := validRFC3339("check at", c.At); err != nil {
		return err
	}
	contradicted := c.Outcome == AvoidOutcomeContradicted
	switch {
	case contradicted && (*c.ApplyExit != 0 || *c.PlanExit != 0):
		return fmt.Errorf("outcome contradicted needs apply_exit 0 and plan_exit 0, got %d and %d", *c.ApplyExit, *c.PlanExit)
	case r.Status == AvoidRecordRetired && !contradicted:
		return fmt.Errorf("outcome %q does not retire a rule: only contradicted does", c.Outcome)
	case r.Status == AvoidRecordKept && c.Outcome != AvoidOutcomeRecurred && c.Outcome != AvoidOutcomeInconclusive:
		return fmt.Errorf("outcome %q is not recurred or inconclusive", c.Outcome)
	}
	return nil
}

var checkIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validRFC3339(field, v string) error {
	if _, err := time.Parse(time.RFC3339, v); err != nil {
		return fmt.Errorf("%s %q is not RFC3339", field, v)
	}
	return nil
}

func appendAvoidLedgerRecord(pitfallsDir, cloud string, ledger *AvoidLedger, rec AvoidLedgerRecord) error {
	if err := rec.validate(); err != nil {
		return err
	}
	ledger.Records = append(ledger.Records, rec)
	dir := avoidChecksDir(pitfallsDir)
	return writePitfallsFile(dir, filepath.Join(dir, cloud+".yaml"), cloud, ledger)
}

// ShapeSHA256 hashes a stored shape: sha256 over each file, in byte order
// of name, of name + NUL + content + NUL. The dir may hold only regular
// *.tf files at its top level. The command, the ratchet and the replay all
// use this one function.
func ShapeSHA256(dir string) (string, error) {
	entries, err := os.ReadDir(dir) // sorted by name
	if err != nil {
		return "", fmt.Errorf("read shape: %w", err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("shape %s holds no .tf files", dir)
	}
	h := sha256.New()
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".tf") {
			return "", fmt.Errorf("shape %s may hold only regular .tf files, found %q", dir, e.Name())
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return "", fmt.Errorf("read shape: %w", err)
		}
		h.Write([]byte(e.Name()))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// avoidRuleRe inverts buildAvoidRule's attribute-only form. The greedy
// summary makes the LAST "Do NOT use" clause the one parsed.
var (
	avoidRuleRe = regexp.MustCompile("(?s)^.+ Do NOT use (attribute `[A-Za-z0-9_]+`(?: and attribute `[A-Za-z0-9_]+`)*) on `([A-Za-z0-9_]+)` — observed in scenario \"(?:[^\"\\\\]|\\\\.)*\" to cause the failure above\\.$")
	avoidAttrRe = regexp.MustCompile("attribute `([A-Za-z0-9_]+)`")
)

// ParseAvoidRule returns the resource and every attribute of an avoid
// rule in buildAvoidRule's attribute form. Any resource-type clause, or
// text in any other form, is not-ok: an unparsed rule is never retired.
func ParseAvoidRule(rule string) (resource string, attributes []string, ok bool) {
	m := avoidRuleRe.FindStringSubmatch(rule)
	if m == nil {
		return "", nil, false
	}
	for _, am := range avoidAttrRe.FindAllStringSubmatch(m[1], -1) {
		attributes = append(attributes, am[1])
	}
	return m[2], attributes, true
}

// RetireAvoidPitfall records a check of a mock-learned avoid rule. A
// contradicted outcome appends a retired record and then removes the
// entry; recurred or inconclusive appends a kept record and leaves the
// corpus alone. Every refusal happens before either file is written.
func RetireAvoidPitfall(pitfallsDir, cloud string, req AvoidRetirement) (AvoidLedgerRecord, error) {
	ledger, err := ReadAvoidLedger(pitfallsDir, cloud)
	if err != nil {
		return AvoidLedgerRecord{}, fmt.Errorf("refusing: avoid-check ledger unreadable: %w", err)
	}
	pf, filePath, err := loadCloudPitfalls(pitfallsDir, cloud)
	if err != nil {
		return AvoidLedgerRecord{}, err
	}
	if pf == nil {
		return AvoidLedgerRecord{}, fmt.Errorf("refusing: no pitfalls corpus for %s", cloud)
	}
	idx, err := findRetirableEntry(pf.Pitfalls, req)
	if err != nil {
		return AvoidLedgerRecord{}, fmt.Errorf("refusing: %w", err)
	}
	entry := pf.Pitfalls[idx]

	rec := AvoidLedgerRecord{
		Status:        AvoidRecordKept,
		Resource:      req.Resource,
		Attributes:    req.Attributes,
		LearnedLayer:  MockDeployLayer,
		LayerEvidence: req.LayerEvidence,
		Entry:         &entry,
		Check:         &req.Check,
	}
	if req.Check.Outcome == AvoidOutcomeContradicted {
		rec.Status = AvoidRecordRetired
	}
	if err := rec.validate(); err != nil {
		return AvoidLedgerRecord{}, fmt.Errorf("refusing: %w", err)
	}
	if err := checkStoredShape(pitfallsDir, req); err != nil {
		return AvoidLedgerRecord{}, fmt.Errorf("refusing: %w", err)
	}

	if err := appendAvoidLedgerRecord(pitfallsDir, cloud, ledger, rec); err != nil {
		return AvoidLedgerRecord{}, err
	}
	if rec.Status == AvoidRecordKept {
		return rec, nil
	}
	pf.Pitfalls = slices.Delete(pf.Pitfalls, idx, idx+1)
	return rec, writePitfallsFile(pitfallsDir, filePath, cloud, pf)
}

// findRetirableEntry returns the one corpus entry the check is about. Every
// entry on the resource that names a requested attribute must be it: a
// mock-learned avoid entry forbidding exactly those attributes. Anything
// else naming them would outlive the retirement.
func findRetirableEntry(entries []PitfallEntry, req AvoidRetirement) (int, error) {
	if req.Resource == "" || len(req.Attributes) == 0 || slices.Contains(req.Attributes, "") {
		return 0, errors.New("a resource and at least one attribute are required")
	}
	found := -1
	for i, e := range entries {
		if e.Resource != req.Resource || !namesAnyAttribute(e.Rule, req.Attributes) {
			continue
		}
		resource, attrs, ok := ParseAvoidRule(e.Rule)
		if e.Source != AvoidSource || !ok || resource != req.Resource {
			return 0, fmt.Errorf("entry %d on %s (source %s) names %v but is not an avoid rule in the attribute form", i, e.Resource, e.Source, req.Attributes)
		}
		if e.LearnedLayer != "" && e.LearnedLayer != MockDeployLayer {
			return 0, fmt.Errorf("entry %d on %s was learned at %s, not mock_deploy: mock evidence cannot retire it", i, e.Resource, e.LearnedLayer)
		}
		if e.LearnedLayer == "" && req.LayerEvidence == "" {
			return 0, fmt.Errorf("entry %d on %s has no learned_layer and no layer_evidence establishes it was mock_deploy", i, e.Resource)
		}
		if found >= 0 {
			return 0, fmt.Errorf("entries %d and %d on %s both name %v; retire one rule at a time", found, i, e.Resource, req.Attributes)
		}
		if !sameSet(attrs, req.Attributes) {
			return 0, fmt.Errorf("entry %d on %s forbids %v; a check must cover every attribute, got %v", i, e.Resource, attrs, req.Attributes)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("no corpus entry on %s names %v", req.Resource, req.Attributes)
	}
	return found, nil
}

func namesAnyAttribute(text string, attrs []string) bool {
	return slices.ContainsFunc(attrs, func(a string) bool { return attributeAppearsInDetail(text, a) })
}

func sameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(slices.Compact(x), slices.Compact(y))
}

// checkStoredShape proves the stored shape is the one the check hashed,
// and that it sets every attribute on the resource.
func checkStoredShape(pitfallsDir string, req AvoidRetirement) error {
	dir := avoidShapeDir(pitfallsDir, req.Check.ID)
	sum, err := ShapeSHA256(dir)
	if err != nil {
		return err
	}
	if sum != req.Check.ShapeSHA256 {
		return fmt.Errorf("shape %s hashes to %s, the check recorded %s", dir, sum, req.Check.ShapeSHA256)
	}
	return shapeSetsAttributes(dir, req.Resource, req.Attributes)
}

// shapeSetsAttributes requires one resource block that sets every
// attribute to something other than a literal false, "false", null or "".
// A shape that sets an attribute off does not exercise it.
func shapeSetsAttributes(dir, resource string, attrs []string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return err
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, diags := hclsyntax.ParseConfig(src, filepath.Base(path), hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf("parse shape: %s", diags.Error())
		}
		for _, b := range file.Body.(*hclsyntax.Body).Blocks {
			if b.Type == "resource" && len(b.Labels) == 2 && b.Labels[0] == resource && setsAll(b.Body, attrs) {
				return nil
			}
		}
	}
	return fmt.Errorf("no %s block in the shape sets every attribute of %v", resource, attrs)
}

func setsAll(body *hclsyntax.Body, attrs []string) bool {
	for _, a := range attrs {
		attr, ok := body.Attributes[a]
		if !ok || literalOff(attr.Expr) {
			return false
		}
	}
	return true
}

func literalOff(expr hclsyntax.Expression) bool {
	v, diags := expr.Value(nil)
	if diags.HasErrors() || !v.IsKnown() {
		return false // a reference or call: not a literal
	}
	if v.IsNull() {
		return true
	}
	switch v.Type() {
	case cty.Bool:
		return v.False()
	case cty.String:
		return v.AsString() == "" || v.AsString() == "false"
	}
	return false
}

// retiredHit is a retired attribute a rule names, and the check that
// retired it.
type retiredHit struct {
	Attribute string
	CheckID   string
}

// retiredAttributesNamed returns the attributes retired on resource that
// text names (snake or camel case), skipping any a later relearned record
// re-admitted. Ledger order is time order: it is append-only.
func (l *AvoidLedger) retiredAttributesNamed(resource, text string) []retiredHit {
	retiredBy := map[string]string{}
	for _, r := range l.Records {
		if r.Resource != resource {
			continue
		}
		for _, a := range r.Attributes {
			switch r.Status {
			case AvoidRecordRetired:
				retiredBy[a] = r.Check.ID
			case AvoidRecordRelearned:
				delete(retiredBy, a)
			}
		}
	}
	var hits []retiredHit
	for a, id := range retiredBy {
		if attributeAppearsInDetail(text, a) {
			hits = append(hits, retiredHit{Attribute: a, CheckID: id})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Attribute < hits[j].Attribute })
	return hits
}

// recurrenceError refuses a mock-learned or unknown-layer candidate that
// names a retired attribute: the mock regressed, and CI's replay of the
// retired record is what should go red, not the corpus.
func recurrenceError(p LearnedPitfall, hits []retiredHit) error {
	var named []string
	retired := map[string]bool{}
	for _, h := range hits {
		named = append(named, fmt.Sprintf("`%s` (retired by check %s)", h.Attribute, h.CheckID))
		retired[h.Attribute] = true
	}
	layer := p.LearnedLayer
	if layer == "" {
		layer = "an unknown layer"
	}
	msg := fmt.Sprintf("refusing %s pitfall on %s learned at %s: it names %s; a mock recurrence of a retired rule is a mock regression",
		pitfallSource(p), p.Resource, layer, strings.Join(named, ", "))
	if _, attrs, ok := ParseAvoidRule(p.Rule); ok {
		var fresh []string
		for _, a := range attrs {
			if !retired[a] {
				fresh = append(fresh, "`"+a+"`")
			}
		}
		if len(fresh) > 0 {
			msg += fmt.Sprintf("; %s, which it also forbids, is not learned", strings.Join(fresh, ", "))
		}
	}
	return errors.New(msg)
}
