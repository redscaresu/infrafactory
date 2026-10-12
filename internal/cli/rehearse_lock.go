package cli

import (
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

// rehearsalLock is what the lock tells prepare: the hashicorp/aws version
// it pins, and the warnings for the report.
type rehearsalLock struct {
	aws      string
	warnings []string
}

// lock parses the root's lock. One that is missing or does not parse, has
// no hashicorp/aws entry, pins aws entries at different versions, pins an
// aws version the gate's path table cannot speak for, or names any other
// provider than the admitted ones, is refused by name.
func (in rehearsalGateInput) lock() (rehearsalLock, []string) {
	path := filepath.Join(in.root, rehearsalLockFile)
	name := in.display(path)
	body, problem := rehearsalParse(path, name)
	if problem != "" {
		return rehearsalLock{}, []string{problem + ": refused: a rehearsal takes every provider version from the root's lock"}
	}
	var lock rehearsalLock
	var problems []string
	versions := map[string]bool{}
	for attr := range body.Attributes {
		problems = append(problems, fmt.Sprintf("%s: %s: refused: a lock holds only provider blocks", name, attr))
	}
	for _, block := range body.Blocks {
		host, version, problem := rehearsalLockEntry(rehearsalAt(name, block.DefRange(), rehearsalAddress(block)), block)
		switch {
		case problem != "":
			problems = append(problems, problem)
		case version != "":
			versions[version], lock.aws = true, version
			if host == "registry.terraform.io" {
				lock.warnings = append(lock.warnings, name+": the hashicorp/aws entry is registry.terraform.io's, which Terraform wrote: moved, import, removed and ephemeral semantics may differ under tofu")
			}
		}
	}
	switch len(versions) {
	case 0:
		return lock, append(problems, name+": refused: it has no hashicorp/aws entry the gate can read, and a rehearsal is AWS-only")
	case 1:
		problem, warning := rehearsalAWSVersion(lock.aws)
		if problem != "" {
			problems = append(problems, name+": refused: "+problem)
		}
		if warning != "" {
			lock.warnings = append(lock.warnings, name+": "+warning)
		}
		return lock, problems
	}
	return lock, append(problems, fmt.Sprintf("%s: refused: its hashicorp/aws entries pin different versions (%s)", name, strings.Join(slices.Sorted(maps.Keys(versions)), ", ")))
}

// rehearsalLockEntry reads one lock entry: its host and, for hashicorp/aws,
// its version ("" for another admitted provider), or why it is refused.
func rehearsalLockEntry(at string, block *hclsyntax.Block) (host, version, problem string) {
	if block.Type != "provider" || len(block.Labels) != 1 {
		return "", "", at + ": refused: a lock holds only provider blocks with one label"
	}
	host, source, _ := strings.Cut(strings.ToLower(block.Labels[0]), "/")
	typ, admitted := rehearsalSource(source)
	switch {
	case !rehearsalLockHosts[host] || !admitted:
		return "", "", fmt.Sprintf("%s: refused: the lock names provider %q, and a rehearsal admits only hashicorp/{%s} from %s", at, block.Labels[0],
			strings.Join(slices.Sorted(maps.Keys(rehearsalProviders)), ", "), strings.Join(slices.Sorted(maps.Keys(rehearsalLockHosts)), " or "))
	case typ != "aws":
		return host, "", ""
	}
	attr, ok := block.Body.Attributes["version"]
	if ok {
		version, ok = rehearsalLiteral(attr.Expr)
	}
	if !ok || version == "" {
		return "", "", at + ": refused: its version is not a string literal the gate can read"
	}
	return host, version, ""
}

// rehearsalAWSVersion is why a locked hashicorp/aws version is refused,
// or the report's warning for one that is not harness.AWSProviderVersion.
// rehearsalPathArgs names the arguments that read a local file in
// harness.AWSProviderVersion only, so a later release, or another major,
// could read one through an argument the table does not name: it is
// refused. An earlier release of the same major is admitted, because the
// provider removes an argument only in a major release (unverified;
// ADR-0042 § The lock).
func rehearsalAWSVersion(version string) (problem, warning string) {
	locked, ok := rehearsalRelease(version)
	pinned, _ := rehearsalRelease(harness.AWSProviderVersion)
	switch {
	case !ok:
		return fmt.Sprintf("hashicorp/aws %q is not an exact X.Y.Z release the gate can compare", version), ""
	case locked[0] != pinned[0] || slices.Compare(locked[:], pinned[:]) > 0:
		return fmt.Sprintf("hashicorp/aws %s is later than %s or of another major, and the gate's table of the aws arguments that read a local file is %s's, so it cannot name an argument %s reads one through",
			version, harness.AWSProviderVersion, harness.AWSProviderVersion, version), ""
	case version != harness.AWSProviderVersion:
		return "", fmt.Sprintf("the lock pins hashicorp/aws %s, not %s, the release fakeaws is proven against: a failure under it is a fakeaws gap, to file as a fakeaws story",
			version, harness.AWSProviderVersion)
	}
	return "", ""
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
