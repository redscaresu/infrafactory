package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// prepare copies dir and its local modules into a new rehearsal under the
// current directory's repository, then reads the copy's lock and runs
// gate 1 over the copy, the first init, gate 2 and the second init
// (ADR-0042 § The layout), then stands the walls up. It returns the
// rehearsal's id, the copied root and what the lock tells the report. On a
// refusal the half-made rehearsal dir is removed, unless it is
// errRehearsalKept, which then names the rehearsal and its record.
func (p rehearsePrepare) prepare(ctx context.Context, dir, account, region, varFile string) (result rehearsePrepareResult, err error) {
	repo, err := rehearsalRepo(p.wd)
	if err != nil {
		return result, err
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(p.wd, dir)
	}
	if varFile != "" && !filepath.IsAbs(varFile) {
		varFile = filepath.Join(p.wd, varFile)
	}
	root, problem := rehearsalInside(dir, repo)
	if problem != "" {
		return result, fmt.Errorf("%s is refused: it %s, and a rehearsal copies only code inside the current directory's repository", dir, problem)
	}
	if err := rehearsalVarFileProblem(varFile); err != nil {
		return result, err
	}
	modules, err := rehearsalLocalModuleSources(root, repo)
	if err != nil {
		return result, err
	}
	ancestor := root
	for _, module := range append([]string{root}, modules...) {
		// The copy leaves these dirs out, and the rehearsals live in one.
		rel, _ := filepath.Rel(repo, module)
		for _, elem := range strings.Split(rel, string(filepath.Separator)) {
			if rehearsalCopyExcluded(elem) {
				return result, fmt.Errorf("%s is refused: it lies in %s, which the copy leaves out", module, elem)
			}
		}
		for !rehearsalUnder(module, ancestor) {
			ancestor = filepath.Dir(ancestor)
		}
	}
	rehearsals, err := rehearsalsDir(repo)
	if err != nil {
		return result, err
	}

	id := p.newID()
	rehearsal := filepath.Join(rehearsals, id)
	if err := os.Mkdir(rehearsal, 0o700); err != nil {
		return result, fmt.Errorf("rehearsal %s is refused: it cannot be made new: %w", rehearsal, err)
	}
	defer func() {
		switch {
		case err == nil:
		case errors.Is(err, errRehearsalKept):
			err = fmt.Errorf("%w: rehearsal %s, record %s", err, id, filepath.Join(rehearsal, rehearsalRecordFile))
		default:
			_ = os.RemoveAll(rehearsal)
		}
	}()
	in, err := rehearsalCopy(rehearsal, repo, ancestor, root, varFile)
	if err != nil {
		return result, err
	}
	in.region = region
	lock, aliases, err := p.gateAndInit(ctx, rehearsal, in)
	if err != nil {
		return result, err
	}
	workRel, _ := filepath.Rel(rehearsal, in.boundary)
	rootRel, _ := filepath.Rel(rehearsal, in.root)
	record := rehearsalRecord{Account: account, Region: region, Roots: []rehearsalRoot{{WorkDir: workRel, Root: rootRel, AWSAliases: aliases}}}
	if err := p.walls(ctx, rehearsal, record); err != nil {
		return result, err
	}
	return rehearsePrepareResult{ID: id, WorkDir: in.root, AWSVersion: lock.aws(), Warnings: lock.warnings}, nil
}

// rehearsalCopy copies ancestor into <rehearsal>/work at its path relative
// to repo, so a module source that climbs above ancestor and back down
// resolves in the copy as it does in the repo, and the var file to
// <rehearsal>/vars.tfvars, outside work so tofu never auto-loads it. It
// returns gate 1's input over the copy.
func rehearsalCopy(rehearsal, repo, ancestor, root, varFile string) (rehearsalGateInput, error) {
	work := filepath.Join(rehearsal, "work")
	rel, _ := filepath.Rel(repo, ancestor)
	dst := filepath.Join(work, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return rehearsalGateInput{}, err
	}
	if err := rehearsalCopyTree(ancestor, dst); err != nil {
		return rehearsalGateInput{}, err
	}
	rootRel, _ := filepath.Rel(repo, root)
	in := rehearsalGateInput{root: filepath.Join(work, rootRel), boundary: work}
	if varFile == "" {
		return in, nil
	}
	in.varFile = filepath.Join(rehearsal, "vars.tfvars")
	return in, rehearsalCopyFile(varFile, in.varFile)
}

// rehearsalVarFileProblem refuses a --var-file the gate cannot parse as
// .tfvars: a .json one, or anything but a regular file.
func rehearsalVarFileProblem(varFile string) error {
	if varFile == "" {
		return nil
	}
	if strings.HasSuffix(strings.ToLower(varFile), ".json") {
		return fmt.Errorf("--var-file %s is refused: a .json var file is not parsed by the gate", varFile)
	}
	info, err := os.Stat(varFile)
	if err != nil {
		return fmt.Errorf("--var-file %s is refused: %w", varFile, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("--var-file %s is refused: it is not a regular file", varFile)
	}
	return nil
}

// rehearsalRepo is the nearest ancestor of wd, resolved, holding .git (a
// dir, or a worktree's file).
func rehearsalRepo(wd string) (string, error) {
	dir, err := rehearsalResolve(wd)
	if err != nil {
		return "", fmt.Errorf("the current directory %s does not resolve: %w", wd, err)
	}
	for {
		git := filepath.Join(dir, ".git")
		info, err := os.Lstat(git)
		switch {
		case err == nil && (info.IsDir() || info.Mode().IsRegular()):
			return dir, nil
		case err == nil:
			return "", fmt.Errorf("%s is refused: it is neither a directory nor a worktree file", git)
		case !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("cannot look for the repository at %s: %w", git, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("the current directory %s is refused: it is not inside a git repository, and a rehearsal lives at the root of the one that holds it", wd)
		}
		dir = parent
	}
}

// rehearsalsDir makes <repo>/.infrafactory/rehearsals, 0700, holding a
// .gitignore of *. A symlink, or anything but a dir, at either level is
// refused: the rehearsals stay in this repository.
func rehearsalsDir(repo string) (string, error) {
	dir := repo
	for _, name := range []string{".infrafactory", "rehearsals"} {
		dir = filepath.Join(dir, name)
		if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%s is refused: it is not a directory, and a symlink would put the rehearsals outside this repository", dir)
		}
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, gitignoreAll(dir)
}

// gitignoreAll writes a .gitignore of * into dir, so git ignores
// everything in it whichever repository holds it. It is written beside
// and renamed over, so an existing .gitignore that is a symlink is
// replaced, never written through.
func gitignoreAll(dir string) error {
	return writeSynced(filepath.Join(dir, ".gitignore"), []byte("*\n"))
}

// rehearsalCopyExcluded is a name left out of the copy: tofu's data dir,
// git's, infrafactory's own (which holds the rehearsals) and state files.
// Names compare lowercased, as a case-insensitive filesystem does.
func rehearsalCopyExcluded(name string) bool {
	lower := strings.ToLower(name)
	return lower == ".terraform" || lower == ".git" || lower == ".infrafactory" || strings.Contains(lower, ".tfstate")
}

// rehearsalCopyTree copies src to dst, dirs 0700 and files 0600, leaving
// out what rehearsalCopyExcluded names. A symlink, or anything else but a
// dir or a regular file, is refused.
func rehearsalCopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != src && rehearsalCopyExcluded(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.Mkdir(target, 0o700)
		case d.Type().IsRegular():
			return rehearsalCopyFile(path, target)
		}
		return fmt.Errorf("%s is refused: it is a symlink or another non-regular file, and the copy admits only directories and regular files", path)
	})
}

// rehearsalCopyFile copies src to a new file dst, 0600.
func rehearsalCopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
