package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// work is the copy inside a module fixture's temp dir, two levels down so
// a source can climb out of it to a dir that exists.
const work = "repo/work/"

// moduleCopy writes files and symlinks (paths relative to a fresh temp
// dir, link: target) and returns the temp dir and the gate input for the
// copy at work, whose root is the copy itself.
func moduleCopy(t *testing.T, files, symlinks map[string]string) (string, rehearsalGateInput) {
	t.Helper()
	tmp := t.TempDir()
	for name, content := range files {
		path := filepath.Join(tmp, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	for link, target := range symlinks {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(tmp, link)), 0o700))
		require.NoError(t, os.Symlink(filepath.Join(tmp, target), filepath.Join(tmp, link)))
	}
	copyDir := filepath.Join(tmp, work)
	return tmp, rehearsalGateInput{root: copyDir, boundary: copyDir, region: "us-east-1"}
}

func moduleBlock(body string) map[string]string {
	return map[string]string{work + "main.tf": "module \"m\" {\n" + body + "}\n", work + "m/main.tf": manifestOutput}
}

const manifestOutput = "output \"x\" {\n  value = 1\n}\n"

func TestRehearsalGateModuleSources(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		symlinks map[string]string
		want     []string
	}{
		{name: "a registry source with a version", files: moduleBlock("  source  = \"cloudposse/label/null\"\n  version = \"0.25.0\"\n"),
			want: []string{`main.tf:2: module.m.source: source "cloudposse/label/null" is refused`, "main.tf:3: module.m.version: refused: a local module takes no version"}},
		{name: "a host-qualified registry source", files: moduleBlock("  source = \"registry.opentofu.org/cloudposse/label/null\"\n"),
			want: []string{`main.tf:2: module.m.source: source "registry.opentofu.org/cloudposse/label/null" is refused`}},
		{name: "a git:: source", files: moduleBlock("  source = \"git::https://github.com/cloudposse/terraform-null-label.git?ref=0.25.0\"\n"),
			want: []string{`main.tf:2: module.m.source: source "git::https://github.com/cloudposse/terraform-null-label.git?ref=0.25.0" is refused`}},
		{name: "a git@ source", files: moduleBlock("  source = \"git@github.com:o/r.git\"\n"),
			want: []string{`main.tf:2: module.m.source: source "git@github.com:o/r.git" is refused`}},
		{name: "a private registry source", files: moduleBlock("  source = \"app.terraform.io/o/m/aws\"\n"),
			want: []string{`main.tf:2: module.m.source: source "app.terraform.io/o/m/aws" is refused`}},
		{name: "an absolute path, which init downloads", files: moduleBlock("  source = \"/etc\"\n"),
			want: []string{`main.tf:2: module.m.source: source "/etc" is refused`}},
		{name: "a source escaping the copy", files: map[string]string{
			work + "main.tf":           "module \"m\" {\n  source = \"../../outside-the-repo\"\n}\n",
			"outside-the-repo/main.tf": manifestOutput,
		}, want: []string{`main.tf:2: module.m.source: source "../../outside-the-repo" resolves to `, ", outside the boundary "}},
		{name: "a source through a symlink out of the copy", files: map[string]string{
			work + "main.tf":    "module \"m\" {\n  source = \"./m\"\n}\n",
			"elsewhere/main.tf": manifestOutput,
		}, symlinks: map[string]string{work + "m": "elsewhere"},
			want: []string{`main.tf:2: module.m.source: source "./m" resolves to `, ", outside the boundary "}},
		{name: "a source that does not exist", files: map[string]string{work + "main.tf": "module \"m\" {\n  source = \"./gone\"\n}\n"},
			want: []string{`main.tf:2: module.m.source: source "./gone" does not resolve`}},
		{name: "a source from a variable", files: map[string]string{
			work + "main.tf":   "variable \"src\" {\n  default = \"./m\"\n}\nmodule \"m\" {\n  source = var.src\n}\n",
			work + "m/main.tf": manifestOutput,
		}, want: []string{"main.tf:5: module.m.source: refused: the source is not a literal string (it reads var.src)"}},
		{name: "no source", files: map[string]string{work + "main.tf": "module \"m\" {}\n"},
			want: []string{"main.tf:1: module.m: refused: no source"}},
		{name: "a local source with a version", files: moduleBlock("  source  = \"./m\"\n  version = \"1.0.0\"\n"),
			want: []string{"main.tf:3: module.m.version: refused: a local module takes no version"}},
		{name: "a child's source", files: map[string]string{
			work + "main.tf":   "module \"m\" {\n  source = \"./m\"\n}\n",
			work + "m/main.tf": "module \"r\" {\n  source = \"cloudposse/label/null\"\n}\n",
		}, want: []string{`module "m": m/main.tf:2: module.r.source: source "cloudposse/label/null" is refused`}},
		{name: "a source naming a file", files: moduleBlock("  source = \"./main.tf\"\n"),
			want: []string{`main.tf:2: module.m.source: source "./main.tf" is refused: it is not a directory`}},
		{name: "a module block with no label", files: map[string]string{work + "main.tf": "module {\n  source = \"./m\"\n}\n"},
			want: []string{"main.tf:1: module: a module block takes one label"}},
		// The gate follows each module block itself: in.modules is empty.
		{name: "a provisioner in a module the root calls", files: map[string]string{
			work + "main.tf":   "module \"m\" {\n  source = \"./m\"\n}\n",
			work + "m/main.tf": provisioner,
		}, want: []string{`module "m": m/main.tf:2: terraform_data.t.provisioner.local-exec: "provisioner" blocks are refused`}},
		{name: "a provider aws in a module a module calls", files: map[string]string{
			work + "main.tf":        "module \"m\" {\n  source = \"./m\"\n}\n",
			work + "m/main.tf":      "module \"s\" {\n  source = \"../shared\"\n}\n",
			work + "shared/main.tf": "provider \"aws\" {}\n",
		}, want: []string{`module "m.s": shared/main.tf:1: provider.aws: a provider "aws" block in a child module is refused`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, in := moduleCopy(t, tc.files, tc.symlinks)
			err := rehearsalGate(in)
			require.ErrorIs(t, err, errRehearsalRefuses)
			for _, want := range tc.want {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

const provisioner = "resource \"terraform_data\" \"t\" {\n  provisioner \"local-exec\" {\n    command = \"id\"\n  }\n}\n"

// TestRehearsalGateAdmitsLocalModules: ./m inside the copy, and m's
// ../shared, pass gate 1, and rehearsalLocalModuleSources returns each
// dir the root reaches, under the copy, once. A module cycle ends.
func TestRehearsalGateAdmitsLocalModules(t *testing.T) {
	_, in := moduleCopy(t, map[string]string{
		work + "main.tf":        "module \"m\" {\n  source = \"./m\"\n  name   = \"x\"\n}\nmodule \"again\" {\n  source = \"./m\"\n}\n",
		work + "m/main.tf":      "module \"s\" {\n  source = \"../shared\"\n}\n",
		work + "shared/main.tf": "module \"back\" {\n  source = \"../m\"\n}\n",
	}, nil)
	assert.NoError(t, rehearsalGate(in))

	sources, err := rehearsalLocalModuleSources(in.root, in.boundary)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(in.root, "m"), filepath.Join(in.root, "shared")}, sources)
}

func TestRehearsalLocalModuleSourcesRefuses(t *testing.T) {
	_, in := moduleCopy(t, map[string]string{
		work + "main.tf":   "module \"m\" {\n  source = \"./m\"\n}\nmodule \"g\" {\n  source = \"git::https://example.com/m.git\"\n}\n",
		work + "m/main.tf": manifestOutput,
		work + "broken.tf": "module \"b\" {\n",
	}, nil)
	sources, err := rehearsalLocalModuleSources(in.root, in.boundary)
	require.ErrorIs(t, err, errRehearsalRefuses)
	assert.Nil(t, sources)
	assert.Contains(t, err.Error(), `main.tf:5: module.g.source: source "git::https://example.com/m.git" is refused`)
	assert.Contains(t, err.Error(), "broken.tf: does not parse")
}

// localManifest is the manifest TestAssumption_ModulesManifestShape pinned
// for a root calling ./m, where m calls ../shared.
const localManifest = `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"m","Source":"./m","Dir":"m"},{"Key":"m.s","Source":"../shared","Dir":"shared"}]}`

// localModules is the copy localManifest describes.
func localModules(extra map[string]string) map[string]string {
	files := map[string]string{
		work + "main.tf":        "module \"m\" {\n  source = \"./m\"\n}\n",
		work + "m/main.tf":      "module \"s\" {\n  source = \"../shared\"\n}\n",
		work + "shared/main.tf": manifestOutput,
	}
	for name, content := range extra {
		files[name] = content
	}
	return files
}

func TestRehearsalModulesGate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		symlinks map[string]string
		// manifest is modules.json, $TMP standing for the temp dir; ""
		// writes none.
		manifest string
		want     []string
	}{
		{name: "a fetched module's local child, under <data>/modules", files: map[string]string{
			work + "main.tf":                    "module \"r\" {\n  source = \"./r\"\n}\n",
			work + "r/main.tf":                  manifestOutput,
			"data/modules/registry/sub/main.tf": manifestOutput,
		}, manifest: `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"registry.sub","Source":"./sub","Dir":"$TMP/data/modules/registry/sub"}]}`,
			want: []string{`modules.json: module "registry.sub": refused: dir "$TMP/data/modules/registry/sub" lies under $TMP/data/modules`}},
		{name: "a registry module", files: map[string]string{
			work + "main.tf":                "module \"r\" {\n  source = \"./r\"\n}\n",
			work + "r/main.tf":              manifestOutput,
			"data/modules/registry/main.tf": manifestOutput,
		}, manifest: `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"registry","Source":"registry.opentofu.org/cloudposse/label/null","Dir":"$TMP/data/modules/registry"}]}`,
			want: []string{
				`modules.json: module "registry": refused: source "registry.opentofu.org/cloudposse/label/null"`,
				`modules.json: module "registry": refused: dir "$TMP/data/modules/registry" lies under`,
			}},
		{name: "a local entry escaping the copy", files: localModules(map[string]string{"repo/outside/main.tf": manifestOutput}),
			manifest: `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"m","Source":"./m","Dir":"../outside"}]}`,
			want:     []string{`modules.json: module "m": refused: dir "../outside" resolves to `, ", outside the boundary "}},
		{name: "a local entry at an absolute dir outside the copy", files: localModules(map[string]string{"ext/sub/main.tf": manifestOutput}),
			manifest: `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"e.in","Source":"./sub","Dir":"$TMP/ext/sub"}]}`,
			want:     []string{`modules.json: module "e.in": refused: dir "$TMP/ext/sub" resolves to `}},
		{name: "a fetched module symlinked into the copy", files: localModules(nil),
			symlinks: map[string]string{"data/modules/e": work + "m"},
			manifest: `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"e","Source":"./m","Dir":"$TMP/data/modules/e"}]}`,
			want:     []string{`modules.json: module "e": refused: dir "$TMP/data/modules/e" lies under $TMP/data/modules`}},
		{name: "a local entry whose dir is a symlink out of the copy", files: map[string]string{
			work + "main.tf":    "module \"m\" {\n  source = \"./n\"\n}\n",
			work + "n/main.tf":  manifestOutput,
			"elsewhere/main.tf": manifestOutput,
		}, symlinks: map[string]string{work + "m": "elsewhere"},
			manifest: `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"m","Source":"./m","Dir":"m"}]}`,
			want:     []string{`modules.json: module "m": refused: dir "m" resolves to $TMP/elsewhere, outside the boundary`}},
		{name: "a provisioner in a local module", files: localModules(map[string]string{work + "shared/main.tf": provisioner}),
			manifest: localManifest,
			want:     []string{`module "m.s": shared/main.tf:2: terraform_data.t.provisioner.local-exec: "provisioner" blocks are refused`}},
		{name: "an unparseable manifest", files: localModules(nil), manifest: `{"Modules":[`,
			want: []string{"modules.json: does not parse"}},
		{name: "a missing manifest with module blocks", files: localModules(nil),
			want: []string{"modules.json: refused: the first init wrote no manifest, which only a root with no module blocks may lack"}},
		{name: "no root entry", files: localModules(nil), manifest: `{"Modules":[]}`,
			want: []string{"modules.json: refused: it names 0 root entries, not one"}},
		{name: "a root entry elsewhere", files: localModules(nil), manifest: `{"Modules":[{"Key":"","Source":"","Dir":"m"}]}`,
			want: []string{`modules.json: refused: the root entry has Source "" and Dir "m"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp, in := moduleCopy(t, tc.files, tc.symlinks)
			err := rehearsalModulesGate(in, writeManifest(t, tmp, tc.manifest))
			require.ErrorIs(t, err, errRehearsalRefuses)
			resolved, evalErr := filepath.EvalSymlinks(tmp)
			require.NoError(t, evalErr)
			msg := strings.ReplaceAll(strings.ReplaceAll(err.Error(), resolved, "$TMP"), tmp, "$TMP")
			for _, want := range tc.want {
				assert.Contains(t, msg, want)
			}
		})
	}
}

func TestRehearsalModulesGateAdmits(t *testing.T) {
	t.Run("the local modules the assumption test pinned", func(t *testing.T) {
		tmp, in := moduleCopy(t, localModules(nil), nil)
		assert.NoError(t, rehearsalModulesGate(in, writeManifest(t, tmp, localManifest)))
	})
	t.Run("no manifest for a root with no module blocks", func(t *testing.T) {
		tmp, in := moduleCopy(t, map[string]string{work + "main.tf": manifestOutput}, nil)
		assert.NoError(t, rehearsalModulesGate(in, writeManifest(t, tmp, "")))
	})
}

// writeManifest writes manifest, with $TMP replaced by tmp, as
// <tmp>/data/modules/modules.json, unless it is "", and returns the data dir.
func writeManifest(t *testing.T, tmp, manifest string) string {
	t.Helper()
	data := filepath.Join(tmp, "data")
	if manifest == "" {
		return data
	}
	path := filepath.Join(data, "modules", "modules.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(manifest, "$TMP", tmp)), 0o600))
	return data
}

// TestRehearsalMissingManifestNamesTheCause: with no manifest, a root .tf
// that does not parse is named, not taken for module blocks.
func TestRehearsalMissingManifestNamesTheCause(t *testing.T) {
	tmp, in := moduleCopy(t, map[string]string{work + "main.tf": manifestOutput, work + "broken.tf": "output \"o\" {\n"}, nil)
	_, problems := in.manifestModules(writeManifest(t, tmp, ""))
	assert.Contains(t, strings.Join(problems, "; "), "broken.tf: does not parse")
}
