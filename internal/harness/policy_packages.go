package harness

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/open-policy-agent/opa/ast"
)

var packagePattern = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z0-9_.]+)\s*$`)

func discoverPolicyPackages(policyPaths []string) ([]string, error) {
	seen := make(map[string]struct{})

	for _, path := range policyPaths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat policy path %q: %w", path, err)
		}

		if info.IsDir() {
			err := filepath.WalkDir(path, func(filePath string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() || filepath.Ext(filePath) != ".rego" {
					return nil
				}
				return addPackageFromFile(filePath, seen)
			})
			if err != nil {
				return nil, fmt.Errorf("walk policy dir %q: %w", path, err)
			}
			continue
		}

		if filepath.Ext(path) == ".rego" {
			if err := addPackageFromFile(path, seen); err != nil {
				return nil, err
			}
		}
	}

	packages := make([]string, 0, len(seen))
	for pkg := range seen {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	return packages, nil
}

func addPackageFromFile(path string, seen map[string]struct{}) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read policy file %q: %w", path, err)
	}

	matches := packagePattern.FindSubmatch(payload)
	if len(matches) != 2 {
		return nil
	}
	seen[string(matches[1])] = struct{}{}

	return nil
}

// PolicyFileDefinesRule reports whether one .rego file defines the
// named rule (for example "deny_state").
//
// # Why this has to be asked at all
//
// Rego rules are UNDEFINED rather than false when they do not exist,
// and an undefined rule evaluates to zero results -- which is exactly
// what a defined rule that found nothing wrong returns. So the
// evaluator cannot tell "this policy has no state rule" from "this
// policy's state rule passed", and reports both as green.
//
// That is the false-coverage shape this project keeps removing: a
// scenario names `check: region_restriction` under its acceptance
// criteria, the run reports state_policy pass, and nothing ran.
//
// Takes a FILE rather than a package name, because that is what the
// caller resolved from constraint_policies. A first version took a
// package and was handed the criterion's `check:` value instead --
// "region_restriction" against a package called
// "scaleway.region_restriction" -- so it reported every policy as
// undefined. It never matched, which made the skip fire for a rule
// that was right there.
//
// Answered from the AST, so a rule named in a comment or a string does
// not count as defined.
func PolicyFileDefinesRule(path, rule string) (bool, error) {
	if filepath.Ext(path) != ".rego" {
		return false, fmt.Errorf("not a rego file: %q", path)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read policy file %q: %w", path, err)
	}
	module, err := ast.ParseModule(path, string(payload))
	if err != nil {
		return false, fmt.Errorf("parse policy file %q: %w", path, err)
	}
	for _, r := range module.Rules {
		if r.Head != nil && r.Head.Name.String() == rule {
			return true, nil
		}
	}
	return false, nil
}
