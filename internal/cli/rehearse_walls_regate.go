package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A later step (end, passthrough, against-the-copy) finds a rehearsal
// with rehearsalOpen, and passthrough re-gates a root with rehearsalRegate
// before every tofu step (ADR-0042 § The gate bar for walls).

var (
	errRehearsalBadID   = errors.New("not a rehearsal id")
	errRehearsalMissing = errors.New("no such rehearsal")
	errRehearsalRecord  = errors.New("the rehearsal's record is refused")
)

// openRehearsal is a rehearsal rehearsalOpen found: its dir and its record.
type openRehearsal struct {
	dir    string
	record rehearsalRecord
}

// rehearsalOpen finds rehearsal id in repo, reads its record and dials
// rehearsalDeadProxy again: prepare dials it only before the second init,
// and something may listen there since. Four refusals are errors.Is-able,
// so a caller can act on each: errRehearsalBadID, errRehearsalMissing,
// errRehearsalRecord (the dir is still returned) and errRehearsalProxyLive
// (the dir and record are still returned). Any other error is a failure
// to find the rehearsal. It creates nothing.
func rehearsalOpen(repo, id string) (openRehearsal, error) {
	dir, err := rehearsalFind(repo, id)
	if err != nil {
		return openRehearsal{}, err
	}
	record, err := readRehearsalRecord(dir)
	if err != nil {
		return openRehearsal{dir: dir}, fmt.Errorf("%w: %w", errRehearsalRecord, err)
	}
	opened := openRehearsal{dir: dir, record: record}
	return opened, rehearsalProxyRefuses(context.Background(), "rehearsal "+id, rehearsalDeadProxy)
}

// rehearsalFind is <repo>/.infrafactory/rehearsals/<id>, each level
// checked with Lstat. The id is one path element, not . or ..; a missing
// level is errRehearsalMissing, and an <id> that is not a dir, a symlink
// included, is errRehearsalBadID. It never calls rehearsalsDir, so it
// creates nothing.
func rehearsalFind(repo, id string) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("%w: %q is not one path element", errRehearsalBadID, id)
	}
	dir := repo
	levels := []string{".infrafactory", "rehearsals", id}
	for i, name := range levels {
		dir = filepath.Join(dir, name)
		info, err := os.Lstat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("%w: %s", errRehearsalMissing, id)
		case err != nil:
			return "", fmt.Errorf("cannot look for rehearsal %s: %w", id, err)
		case info.IsDir():
		case i == len(levels)-1:
			return "", fmt.Errorf("%w: %s is not a directory", errRehearsalBadID, dir)
		default:
			return "", fmt.Errorf("%s is refused: it is not a directory, and a symlink would put the rehearsals outside this repository", dir)
		}
	}
	return dir, nil
}

// entry is the record's entry for the prepared root named name, as the
// record names it (work/envs/prod). A name the record does not hold is
// refused.
func (r openRehearsal) entry(name string) (rehearsalRoot, error) {
	i := slices.IndexFunc(r.record.Roots, func(root rehearsalRoot) bool { return root.Root == name })
	if i < 0 {
		return rehearsalRoot{}, fmt.Errorf("root %s: %w: the rehearsal's record names no such root", name, errRehearsalRefuses)
	}
	return r.record.Roots[i], nil
}

// pathsProblem is why root's paths cannot be gated, or "": its copy must
// be a dir under the rehearsal, never the rehearsal itself, and its root
// a dir in the copy, each resolving to itself, so no symlink moves the
// gate's boundary.
func (r openRehearsal) pathsProblem(root rehearsalRoot) string {
	copied := filepath.Join(r.dir, root.Copy)
	if !filepath.IsLocal(root.Copy) || copied == r.dir {
		return fmt.Sprintf("the record names its copy %q, which is not a dir under the rehearsal", root.Copy)
	}
	if problem := rehearsalResolvesInside(copied, r.dir); problem != "" {
		return fmt.Sprintf("its copy %s %s", root.Copy, problem)
	}
	if problem := rehearsalResolvesInside(filepath.Join(r.dir, root.Root), copied); problem != "" {
		return "it " + problem
	}
	return ""
}

// rehearsalResolvesInside is why path is refused, or "": it must lie
// inside boundary and resolve to itself there, through no symlink.
func rehearsalResolvesInside(path, boundary string) string {
	got, problem := rehearsalInside(path, boundary)
	if problem == "" && got != path {
		problem = "resolves through a symlink to " + got
	}
	return problem
}

// ours is infrafactory's own override files for root, by name, with the
// exact bytes prepare writes there: the backend override and the provider
// override at this record's fakeaws port and region.
func (r rehearsalRecord) ours(root rehearsalRoot) map[string][]byte {
	return map[string][]byte{
		ourOverride:          rehearsalBackendOverride,
		rehearsalAWSOverride: r.providerOverride(root),
	}
}

// providerOverride is the provider override's bytes for root, at this
// record's fakeaws port and region: what walls writes and ours admits.
func (r rehearsalRecord) providerOverride(root rehearsalRoot) []byte {
	return rehearsalProviderOverride(rehearsalFakeawsURL(r.Port), r.Region, root.EndpointKeys, root.AWSAliases)
}

// rehearsalOurOverrides is ours for the named root, rebuilt from the
// record, never read back from the files.
func rehearsalOurOverrides(r openRehearsal, name string) (map[string][]byte, error) {
	root, err := r.entry(name)
	if err != nil {
		return nil, err
	}
	return r.record.ours(root), nil
}

// rehearsalRegate is the full gate over the named root before a tofu
// step, rebuilt from the rehearsal: the root and its copy (the boundary)
// from the record, its region, vars.tfvars when present, the aws version
// from the copy's lock, the modules from the manifest under <id>/data,
// and ours, so each of infrafactory's overrides passes only when
// byte-identical to what prepare wrote. Every refusal is
// errRehearsalRefuses.
func rehearsalRegate(r openRehearsal, name string) error {
	root, err := r.entry(name)
	if err != nil {
		return err
	}
	if problem := r.pathsProblem(root); problem != "" {
		return fmt.Errorf("root %s: %w: %s", name, errRehearsalRefuses, problem)
	}
	ours, err := rehearsalOurOverrides(r, name)
	if err != nil {
		return err
	}
	in := rehearsalGateInput{root: filepath.Join(r.dir, root.Root), boundary: filepath.Join(r.dir, root.Copy), region: r.record.Region, ours: ours}
	varFile := filepath.Join(r.dir, rehearsalVarFile)
	switch _, err := os.Lstat(varFile); {
	case err == nil:
		in.varFile = varFile
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("root %s: %w: cannot look for %s: %v", name, errRehearsalRefuses, varFile, err)
	}
	lock, problems := in.lock()
	if err := rehearsalRefusal(problems); err != nil {
		return fmt.Errorf("root %s: %w", name, err)
	}
	in.awsVersion = lock.aws()
	if err := rehearsalModulesGate(in, filepath.Join(r.dir, "data")); err != nil {
		return fmt.Errorf("root %s: %w", name, err)
	}
	return nil
}
