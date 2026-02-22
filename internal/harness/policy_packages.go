package harness

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
