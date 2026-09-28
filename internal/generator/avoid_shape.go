package generator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// `infrafactory pitfalls check-avoid` replays the shape an avoid rule
// forbids against the mock (ADR-0034, amendment 2026-09-28). This file
// establishes everything that replay needs before any tofu call: the
// entry is retirable, its layer is mock_deploy, and the shape sets every
// forbidden attribute.

// AvoidExitNotRun is the exit recorded for a stage that never ran, such
// as the converge plan after a failed apply. It is not an exit any
// process returns.
const AvoidExitNotRun = -1

// AvoidShapeFile is the one file a cut shape is stored as.
const AvoidShapeFile = "main.tf"

// PreparedAvoidCheck is a check ready to replay.
type PreparedAvoidCheck struct {
	Retirement AvoidRetirement
	Shape      map[string][]byte
}

// PrepareAvoidCheck refuses, writing nothing, unless the ledger reads,
// the corpus holds a retirable entry forbidding exactly attrs on
// resource, its layer is mock_deploy (recorded, or established from the
// run artifacts in from), and from sets every attribute.
func PrepareAvoidCheck(pitfallsDir, cloud, from, resource string, attrs []string) (*PreparedAvoidCheck, error) {
	if _, err := ReadAvoidLedger(pitfallsDir, cloud); err != nil {
		return nil, fmt.Errorf("avoid-check ledger unreadable: %w", err)
	}
	pf, _, err := loadCloudPitfalls(pitfallsDir, cloud)
	if err != nil {
		return nil, err
	}
	if pf == nil {
		return nil, fmt.Errorf("no pitfalls corpus for %s", cloud)
	}
	req := AvoidRetirement{Resource: resource, Attributes: attrs}
	if entry, ok := avoidEntryFor(pf.Pitfalls, resource, attrs); ok && entry.LearnedLayer == "" {
		evidence, err := EstablishLegacyLayer(from, entry.DiscoveredFrom, resource, attrs)
		if err != nil {
			return nil, fmt.Errorf("the entry has no learned_layer and its layer is not established: %w", err)
		}
		req.LayerEvidence = evidence
	}
	if _, err := findRetirableEntry(pf.Pitfalls, req); err != nil {
		return nil, err
	}
	shape, err := CutAvoidShape(from, resource, attrs)
	if err != nil {
		return nil, err
	}
	return &PreparedAvoidCheck{Retirement: req, Shape: shape}, nil
}

// avoidEntryFor is the avoid entry whose rule forbids exactly attrs on
// resource. findRetirableEntry decides whether it may go.
func avoidEntryFor(entries []PitfallEntry, resource string, attrs []string) (PitfallEntry, bool) {
	for _, e := range entries {
		r, a, ok := ParseAvoidRule(e.Rule)
		if e.Source == AvoidSource && ok && r == resource && e.Resource == resource && sameSet(a, attrs) {
			return e, true
		}
	}
	return PitfallEntry{}, false
}

// CutAvoidShape cuts from dir every resource block that sets every
// attribute, plus the resource blocks those reference, transitively, as
// one main.tf. A block that sets an attribute to a literal false, null or
// "" does not exercise it and is refused.
func CutAvoidShape(dir, resource string, attrs []string) (map[string][]byte, error) {
	if resource == "" || len(attrs) == 0 || slices.Contains(attrs, "") {
		return nil, errors.New("a resource and at least one attribute are required")
	}
	blocks, err := loadResourceBlocks(dir)
	if err != nil {
		return nil, fmt.Errorf("read shape source %s: %w", dir, err)
	}
	addrs := make([]string, 0, len(blocks))
	for addr := range blocks {
		addrs = append(addrs, addr)
	}
	slices.Sort(addrs)

	var roots []string
	for _, addr := range addrs {
		b := blocks[addr]
		if b.Type != resource {
			continue
		}
		body, err := parseResourceBody(b)
		if err != nil {
			return nil, err
		}
		if !setsEveryAttribute(body, attrs) {
			continue
		}
		for _, a := range attrs {
			if literalOff(body.Attributes[a].Expr) {
				return nil, fmt.Errorf("%s sets %s to a literal off value, which does not exercise it", addr, a)
			}
		}
		roots = append(roots, addr)
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no %s block in %s sets every attribute of %v", resource, dir, attrs)
	}

	keep := map[string]bool{}
	for queue := roots; len(queue) > 0; queue = queue[1:] {
		if addr := queue[0]; !keep[addr] {
			keep[addr] = true
			queue = append(queue, referencesIn(blocks[addr].Body, addrs)...)
		}
	}
	var buf bytes.Buffer
	for _, addr := range addrs {
		if b := blocks[addr]; keep[addr] {
			fmt.Fprintf(&buf, "resource %q %q {\n%s\n}\n\n", b.Type, b.Name, b.Body)
		}
	}
	return map[string][]byte{AvoidShapeFile: append(bytes.TrimRight(buf.Bytes(), "\n"), '\n')}, nil
}

func parseResourceBody(b *resourceBlock) (*hclsyntax.Body, error) {
	src := fmt.Sprintf("resource %q %q {\n%s\n}\n", b.Type, b.Name, b.Body)
	file, diags := hclsyntax.ParseConfig([]byte(src), b.Type+"."+b.Name, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse %s.%s: %s", b.Type, b.Name, diags.Error())
	}
	return file.Body.(*hclsyntax.Body).Blocks[0].Body, nil
}

func setsEveryAttribute(body *hclsyntax.Body, attrs []string) bool {
	for _, a := range attrs {
		if _, ok := body.Attributes[a]; !ok {
			return false
		}
	}
	return true
}

// WriteAvoidShape stores the cut under avoid-checks/shapes/<checkID>/
// and returns the dir and its ShapeSHA256. It refuses an existing dir:
// a stored shape is evidence and is never overwritten.
func WriteAvoidShape(pitfallsDir, checkID string, shape map[string][]byte) (string, string, error) {
	if !checkIDRe.MatchString(checkID) {
		return "", "", fmt.Errorf("check id %q is not a plain name", checkID)
	}
	dir := avoidShapeDir(pitfallsDir, checkID)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", "", err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("store shape: %w", err)
	}
	for name, content := range shape {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			_ = os.RemoveAll(dir)
			return "", "", err
		}
	}
	sum, err := ShapeSHA256(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", "", err
	}
	return dir, sum, nil
}

// ClassifyAvoidFailure is recurred when a failed or drifting replay names
// the resource or an attribute (camelCase-aware), else inconclusive. Both
// keep the rule.
func ClassifyAvoidFailure(detail, resource string, attrs []string) string {
	if strings.Contains(detail, resource) || namesAnyAttribute(detail, attrs) {
		return AvoidOutcomeRecurred
	}
	return AvoidOutcomeInconclusive
}

// EstablishLegacyLayer proves an entry with no learned_layer was learned
// on the mock, from the run artifacts that taught it. from must be
// <runs>/<scenario>/<run>/iterations/<n>/generated with <scenario> equal
// to discoveredFrom; every aws provider block there must route every
// endpoint to a loopback host; and ../iteration.json must hold an apply
// failure whose detail names the resource and every attribute. It
// returns the evidence to record.
func EstablishLegacyLayer(from, discoveredFrom, resource string, attrs []string) (string, error) {
	abs, err := filepath.Abs(from)
	if err != nil {
		return "", err
	}
	iterDir := filepath.Dir(abs)
	iteration := filepath.Base(iterDir)
	runDir := filepath.Dir(filepath.Dir(iterDir))
	scenario, runID := filepath.Base(filepath.Dir(runDir)), filepath.Base(runDir)
	if _, err := strconv.Atoi(iteration); filepath.Base(abs) != "generated" || err != nil || filepath.Base(filepath.Dir(iterDir)) != "iterations" {
		return "", fmt.Errorf("%s is not a run's <scenario>/<run>/iterations/<n>/generated dir", from)
	}
	if scenario != discoveredFrom {
		return "", fmt.Errorf("run scenario %q is not the entry's discovered_from %q", scenario, discoveredFrom)
	}
	if err := loopbackEndpoints(abs); err != nil {
		return "", err
	}
	if err := applyFailureNames(filepath.Join(iterDir, "iteration.json"), resource, attrs); err != nil {
		return "", err
	}
	return fmt.Sprintf("run %s/%s iteration %s: its apply failure names %s and %s, and every aws endpoint was loopback",
		scenario, runID, iteration, resource, strings.Join(attrs, ", ")), nil
}

// loopbackEndpoints requires at least one provider "aws" block in dir,
// and that each has an endpoints block whose every URL is loopback.
func loopbackEndpoints(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return err
	}
	providers := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, diags := hclsyntax.ParseConfig(src, filepath.Base(path), hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf("parse %s: %s", path, diags.Error())
		}
		for _, b := range file.Body.(*hclsyntax.Body).Blocks {
			if b.Type != "provider" || len(b.Labels) != 1 || b.Labels[0] != "aws" {
				continue
			}
			providers++
			if err := endpointsAreLoopback(b.Body); err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(path), err)
			}
		}
	}
	if providers == 0 {
		return fmt.Errorf("no provider \"aws\" block in %s", dir)
	}
	return nil
}

func endpointsAreLoopback(provider *hclsyntax.Body) error {
	var endpoints *hclsyntax.Body
	for _, b := range provider.Blocks {
		if b.Type == "endpoints" {
			endpoints = b.Body
		}
	}
	if endpoints == nil || len(endpoints.Attributes) == 0 {
		return errors.New("provider \"aws\" has no endpoints block: the apply may have gone to real AWS")
	}
	for name, attr := range endpoints.Attributes {
		v, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || !v.IsKnown() || v.IsNull() || v.Type() != cty.String {
			return fmt.Errorf("endpoint %s is not a literal URL", name)
		}
		u, err := url.Parse(v.AsString())
		if err != nil || !isLoopbackHost(u.Hostname()) {
			return fmt.Errorf("endpoint %s = %q is not on a loopback host", name, v.AsString())
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func applyFailureNames(path, resource string, attrs []string) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read iteration record: %w", err)
	}
	var iteration struct {
		Failures []struct {
			Check  string `json:"check"`
			Detail string `json:"detail"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(payload, &iteration); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for _, f := range iteration.Failures {
		if f.Check == "apply" && strings.Contains(f.Detail, resource) && namesEveryAttribute(f.Detail, attrs) {
			return nil
		}
	}
	return fmt.Errorf("no apply failure in %s names %s and every attribute of %v", path, resource, attrs)
}

func namesEveryAttribute(text string, attrs []string) bool {
	for _, a := range attrs {
		if !attributeAppearsInDetail(text, a) {
			return false
		}
	}
	return true
}
