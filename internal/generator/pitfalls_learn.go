package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LearnedPitfall represents a pitfall discovered from run feedback.
type LearnedPitfall struct {
	Resource       string
	Rule           string
	DiscoveredFrom string // scenario name
}

var (
	// Matches Terraform resource names like scaleway_redis_cluster or
	// google_compute_instance. Without the google_ branch, GCP-side
	// auto-learning never extracts a resource and (after the cross-cloud
	// guard added in the run loop) silently produces zero pitfalls.
	resourceNameRe = regexp.MustCompile(`((?:scaleway|google)_\w+)`)

	// Matches "Unsupported argument" errors with argument name in quotes.
	unsupportedArgRe = regexp.MustCompile(`Unsupported argument.*"(\w+)"`)

	// Matches "at least one of" constraint errors.
	atLeastOneOfRe = regexp.MustCompile(`at least one of\s+(.+?)(?:\s+must|\s+should|\s*\()`)

	// Generic errors that are not learnable.
	genericPatterns = []string{
		"test checks failed",
		"validation failed",
		"exit status",
		"context deadline exceeded",
		"resource with ID",
		"resource not found",
		"command failed",
		"connection refused",
		"timeout",
	}
)

// ExtractLearnedPitfall analyzes a failure detail string and extracts
// a pitfall rule if the error is specific enough to be useful.
// Returns nil if the error is too vague (e.g., "test checks failed").
func ExtractLearnedPitfall(failureDetail, scenarioName string) *LearnedPitfall {
	if failureDetail == "" {
		return nil
	}

	lower := strings.ToLower(failureDetail)

	// Check specific patterns BEFORE generic rejection, since specific
	// errors may be embedded inside generic wrappers like "exit status 1 | stderr: ..."

	// Try specific pattern: password constraint.
	if strings.Contains(lower, "password does not respect constraint") ||
		strings.Contains(lower, "password") && strings.Contains(lower, "complexity") {
		// Only fall back to the Scaleway-flavoured default when the
		// failure detail names no resource at all. This used to be a
		// blind `scaleway_redis_cluster` default that, after the
		// cross-cloud guard in run_command.go, silently dropped the
		// learning on GCP. Now: extract whichever cloud's resource is
		// in the detail; otherwise skip rather than fabricate.
		resource := extractResource(failureDetail)
		if resource == "" {
			return nil
		}
		return &LearnedPitfall{
			Resource:       resource,
			Rule:           "The password must meet the provider's complexity requirements (minimum length, uppercase, lowercase, digit, and special character).",
			DiscoveredFrom: scenarioName,
		}
	}

	// Try specific pattern: K8s version / auto_upgrade mismatch.
	if strings.Contains(lower, "minor version") && strings.Contains(lower, "auto upgrade") ||
		strings.Contains(lower, "auto_upgrade") && strings.Contains(lower, "version") {
		resource := extractResource(failureDetail)
		if resource == "" {
			return nil
		}
		return &LearnedPitfall{
			Resource:       resource,
			Rule:           "Version and auto_upgrade MUST be consistent: WITHOUT auto_upgrade use a full patch version like \"1.31.2\"; WITH auto_upgrade enabled use ONLY a minor version like \"1.31\".",
			DiscoveredFrom: scenarioName,
		}
	}

	// Try specific pattern: Unsupported argument.
	if matches := unsupportedArgRe.FindStringSubmatch(failureDetail); len(matches) >= 2 {
		resource := extractResource(failureDetail)
		argName := matches[1]
		if resource != "" {
			return &LearnedPitfall{
				Resource:       resource,
				Rule:           fmt.Sprintf("The argument %q is not supported. Do not use it.", argName),
				DiscoveredFrom: scenarioName,
			}
		}
	}

	// Try specific pattern: "at least one of".
	if matches := atLeastOneOfRe.FindStringSubmatch(failureDetail); len(matches) >= 2 {
		resource := extractResource(failureDetail)
		constraint := strings.TrimSpace(matches[1])
		if resource != "" {
			return &LearnedPitfall{
				Resource:       resource,
				Rule:           fmt.Sprintf("At least one of %s must be set.", constraint),
				DiscoveredFrom: scenarioName,
			}
		}
	}

	// Reject remaining generic errors that are not actionable.
	for _, pattern := range genericPatterns {
		if strings.Contains(lower, pattern) {
			return nil
		}
	}

	// Fallback: if we can extract a resource and the error is long enough
	// to be meaningful, capture it as a general rule.
	resource := extractResource(failureDetail)
	if resource != "" && len(failureDetail) > 40 {
		// Truncate excessively long details.
		rule := failureDetail
		if len(rule) > 300 {
			rule = rule[:297] + "..."
		}
		return &LearnedPitfall{
			Resource:       resource,
			Rule:           rule,
			DiscoveredFrom: scenarioName,
		}
	}

	return nil
}

// extractResource finds a Terraform resource name in the text. Matches
// scaleway_* or google_* prefixes (mockway / fakegcp providers).
func extractResource(text string) string {
	if match := resourceNameRe.FindString(text); match != "" {
		return match
	}
	return ""
}

// AppendPitfall appends a learned pitfall to the YAML file if it doesn't
// already exist (deduplication by resource + similar rule text).
func AppendPitfall(pitfallsDir, cloud string, pitfall LearnedPitfall) error {
	if pitfallsDir == "" || cloud == "" {
		return nil
	}

	filePath := filepath.Join(pitfallsDir, cloud+".yaml")

	// Ensure directory exists.
	if err := os.MkdirAll(pitfallsDir, 0o755); err != nil {
		return fmt.Errorf("create pitfalls directory: %w", err)
	}

	// Load existing file or start fresh.
	var pf PitfallsFile
	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read pitfalls file: %w", err)
		}
		pf = PitfallsFile{Provider: cloud}
	} else {
		if err := yaml.Unmarshal(data, &pf); err != nil {
			return fmt.Errorf("parse pitfalls file: %w", err)
		}
	}

	// Deduplication: check if a similar pitfall already exists.
	if isDuplicate(pf.Pitfalls, pitfall) {
		return nil
	}

	// Append the new pitfall.
	pf.Pitfalls = append(pf.Pitfalls, PitfallEntry{
		Resource:       pitfall.Resource,
		Rule:           pitfall.Rule,
		Source:         "learned",
		DiscoveredFrom: pitfall.DiscoveredFrom,
	})

	// Marshal and write atomically via temp file.
	out, err := yaml.Marshal(&pf)
	if err != nil {
		return fmt.Errorf("marshal pitfalls: %w", err)
	}

	// Use os.CreateTemp so two concurrent learn-paths racing on the
	// same provider can't clobber each other's tmp file before either
	// rename completes. Mirrors the editPitfalls API handler.
	tmp, err := os.CreateTemp(pitfallsDir, cloud+"-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp pitfalls file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupPath := tmpPath
	defer func() {
		if cleanupPath != "" {
			_ = os.Remove(cleanupPath)
		}
	}()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp pitfalls file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp pitfalls file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp pitfalls file: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("rename pitfalls file: %w", err)
	}
	cleanupPath = ""

	return nil
}

// isDuplicate returns true if any existing pitfall has the same resource
// and shares 3+ significant words with the new rule.
func isDuplicate(existing []PitfallEntry, candidate LearnedPitfall) bool {
	candidateWords := significantWords(candidate.Rule)
	for _, entry := range existing {
		if entry.Resource != candidate.Resource {
			continue
		}
		existingWords := significantWords(entry.Rule)
		shared := 0
		for w := range candidateWords {
			if existingWords[w] {
				shared++
			}
		}
		if shared >= 3 {
			return true
		}
	}
	return false
}

// significantWords extracts lowercase words of 4+ characters from text,
// excluding common stop words.
func significantWords(text string) map[string]bool {
	stopWords := map[string]bool{
		"that": true, "this": true, "with": true, "from": true,
		"have": true, "will": true, "when": true, "must": true,
		"does": true, "should": true, "would": true, "could": true,
		"than": true, "also": true, "into": true, "only": true,
		"each": true, "such": true, "they": true, "been": true,
		"like": true, "then": true, "some": true, "your": true,
		"them": true, "more": true, "make": true, "very": true,
	}

	words := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(text)) {
		// Strip punctuation.
		word = strings.Trim(word, ".,;:!?\"'`()[]{}/<>")
		if len(word) >= 4 && !stopWords[word] {
			words[word] = true
		}
	}
	return words
}
