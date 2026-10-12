package cli

import (
	"cmp"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// The lock rule (ADR-0042 § The lock). Tofu takes every provider version
// from the root's lock, so prepare parses it before gate 1 and again after
// the first init, and refuses what it does not name.

const rehearsalLockFile = ".terraform.lock.hcl"

// rehearsalLockHosts are the registries a lock entry may name. Tofu
// installs a registry.terraform.io entry's version from its own registry
// (TestAssumption_RegistryTerraformIoLockInitialises).
var rehearsalLockHosts = map[string]bool{"registry.opentofu.org": true, "registry.terraform.io": true}

// rehearsalLock is what the lock tells prepare: the version it pins for
// each provider type, and the warnings for the report.
type rehearsalLock struct {
	versions map[string]string
	warnings []string
}

// aws is the hashicorp/aws version the lock pins.
func (l rehearsalLock) aws() string { return l.versions["aws"] }

// lock parses the root's lock. One that is missing or does not parse, has
// no hashicorp/aws entry, pins one provider's entries at different
// versions, pins a provider at a version the gate's path table cannot
// speak for, or names any other provider than the admitted ones, is
// refused by name.
func (in rehearsalGateInput) lock() (rehearsalLock, []string) {
	path := filepath.Join(in.root, rehearsalLockFile)
	name := in.display(path)
	body, problem := rehearsalParse(path, name)
	if problem != "" {
		return rehearsalLock{}, []string{problem + ": refused: a rehearsal takes every provider version from the root's lock"}
	}
	var problems []string
	lock, versions := rehearsalLock{versions: map[string]string{}}, map[string]map[string]bool{}
	for attr := range body.Attributes {
		problems = append(problems, fmt.Sprintf("%s: %s: refused: a lock holds only provider blocks", name, attr))
	}
	for _, block := range body.Blocks {
		host, typ, version, problem := rehearsalLockEntry(rehearsalAt(name, block.DefRange(), rehearsalAddress(block)), block)
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		if versions[typ] == nil {
			versions[typ] = map[string]bool{}
		}
		versions[typ][version], lock.versions[typ] = true, version
		if typ == "aws" && host == "registry.terraform.io" {
			lock.warnings = append(lock.warnings, name+": the hashicorp/aws entry is registry.terraform.io's, which Terraform wrote: moved, import, removed and ephemeral semantics may differ under tofu")
		}
	}
	for _, typ := range slices.Sorted(maps.Keys(versions)) {
		if len(versions[typ]) > 1 {
			problems = append(problems, fmt.Sprintf("%s: refused: its hashicorp/%s entries pin different versions (%s)", name, typ, strings.Join(slices.Sorted(maps.Keys(versions[typ])), ", ")))
		} else if problem := rehearsalReleaseProblem(typ, lock.versions[typ]); problem != "" {
			problems = append(problems, name+": refused: "+problem)
		}
	}
	switch aws := lock.aws(); {
	case aws == "":
		problems = append(problems, name+": refused: it has no hashicorp/aws entry the gate can read, and a rehearsal is AWS-only")
	case aws != harness.AWSProviderVersion:
		lock.warnings = append(lock.warnings, fmt.Sprintf("%s: the lock pins hashicorp/aws %s, not %s, the release fakeaws is proven against: a failure under it is a fakeaws gap, to file as a fakeaws story",
			name, aws, harness.AWSProviderVersion))
	}
	return lock, problems
}

// rehearsalLockEntry reads one lock entry: its host, provider type and
// version, or why it is refused.
func rehearsalLockEntry(at string, block *hclsyntax.Block) (host, typ, version, problem string) {
	if block.Type != "provider" || len(block.Labels) != 1 {
		return "", "", "", at + ": refused: a lock holds only provider blocks with one label"
	}
	host, source, _ := strings.Cut(strings.ToLower(block.Labels[0]), "/")
	typ, admitted := rehearsalSource(source)
	if !rehearsalLockHosts[host] || !admitted {
		return "", "", "", fmt.Sprintf("%s: refused: the lock names provider %q, and a rehearsal admits only hashicorp/{%s} from %s", at, block.Labels[0],
			strings.Join(slices.Sorted(maps.Keys(rehearsalProviders)), ", "), strings.Join(slices.Sorted(maps.Keys(rehearsalLockHosts)), " or "))
	}
	attr, ok := block.Body.Attributes["version"]
	if ok {
		version, ok = rehearsalLiteral(attr.Expr)
	}
	if !ok || version == "" {
		return "", "", "", at + ": refused: its version is not a string literal the gate can read"
	}
	return host, typ, version, ""
}

// rehearsalReleaseProblem is why a locked version of a provider whose
// arguments rehearsalPathArgs names is refused, or "". The table names
// the file-reading arguments of rehearsalPathArgReleases[typ] only, so a
// later release, or another major, could read a file through an argument
// it does not name: it is refused. An earlier release of the same major is
// admitted, because a provider removes an argument only in a major release
// (unverified; ADR-0042 § The lock).
func rehearsalReleaseProblem(typ, version string) string {
	bound, ok := rehearsalPathArgReleases[typ]
	if !ok {
		return ""
	}
	locked, ok := rehearsalRelease(version)
	table, _ := rehearsalRelease(bound)
	switch {
	case !ok:
		return fmt.Sprintf("hashicorp/%s %q is not an exact X.Y.Z release the gate can compare", typ, version)
	case locked[0] != table[0] || slices.Compare(locked[:], table[:]) > 0:
		return fmt.Sprintf("hashicorp/%s %s is later than %s or of another major, and the gate's table of the %s arguments that read a local file is %s's, so it cannot name an argument %s reads one through",
			typ, version, bound, typ, bound, version)
	}
	return ""
}

// rehearsalLockChanges names every provider type that after pins at
// another version than before: a changed version, or an entry after adds
// ("none" before). An entry after lacks passes: dropping one brings in no
// version the user did not lock.
func rehearsalLockChanges(before, after rehearsalLock) []string {
	var changes []string
	for _, typ := range slices.Sorted(maps.Keys(after.versions)) {
		if from, to := before.versions[typ], after.versions[typ]; from != to {
			changes = append(changes, fmt.Sprintf("hashicorp/%s from %s to %s", typ, cmp.Or(from, "none"), to))
		}
	}
	return changes
}

// rehearsalRelease is an exact X.Y.Z release version's numbers. A
// pre-release, a build suffix or a leading zero is not one.
func rehearsalRelease(version string) ([3]int, bool) {
	var release [3]int
	parts := strings.Split(version, ".")
	if len(parts) != len(release) {
		return release, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || part != strconv.Itoa(n) {
			return release, false
		}
		release[i] = n
	}
	return release, true
}
