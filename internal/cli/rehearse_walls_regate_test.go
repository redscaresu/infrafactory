package cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// regateRoot is the root a fake-runner prepare of envs/prod names in its
// record.
var regateRoot = filepath.Join("work", "envs", "prod")

// regatePrepared runs a fake-runner prepare of envs/prod, with args, and
// returns the repo and the rehearsal rehearsalOpen finds.
func regatePrepared(t *testing.T, args ...string) (string, openRehearsal) {
	t.Helper()
	repo := lockRepo(t, prepareLock, "")
	mustWriteFile(t, filepath.Join(repo, "prod.tfvars"), "instance_type = \"t3.micro\"\n")
	run := runPrepareWith(t, stubWalls(rehearsePrepare{}), repo, func(context.Context, harness.Command) (harness.CommandResult, error) {
		return harness.CommandResult{}, nil
	}, args...)
	require.NoError(t, run.err, run.stdout)
	r, err := rehearsalOpen(repo, prepareID)
	require.NoError(t, err)
	return repo, r
}

// rewriteRecord writes record over r's record and opens the rehearsal
// again.
func rewriteRecord(t *testing.T, repo string, r openRehearsal, record rehearsalRecord) openRehearsal {
	t.Helper()
	require.NoError(t, record.write(r.dir))
	r, err := rehearsalOpen(repo, prepareID)
	require.NoError(t, err)
	return r
}

// After a fake-runner prepare, rehearsalOpen finds its dir and record,
// and the re-gate passes for its one root, var file included.
func TestRehearsalRegatePassesAfterPrepare(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"--var-file", "prod.tfvars"}} {
		repo, r := regatePrepared(t, args...)

		assert.Equal(t, preparedDir(repo), r.dir)
		record, err := readRehearsalRecord(preparedDir(repo))
		require.NoError(t, err)
		assert.Equal(t, record, r.record)
		require.Len(t, r.record.Roots, 1)
		assert.Equal(t, regateRoot, r.record.Roots[0].Root)
		assert.NoError(t, rehearsalRegate(r, regateRoot), args)
	}
}

// The re-gate reads vars.tfvars when prepare copied one: a reference
// planted there is refused.
func TestRehearsalRegateReadsTheVarFile(t *testing.T) {
	t.Parallel()
	_, r := regatePrepared(t, "--var-file", "prod.tfvars")
	require.NoError(t, os.WriteFile(filepath.Join(r.dir, rehearsalVarFile), []byte("instance_type = var.x\n"), 0o600))

	err := rehearsalRegate(r, regateRoot)

	require.ErrorIs(t, err, errRehearsalRefuses)
	assert.Contains(t, err.Error(), "a .tfvars value must be a literal")
}

// Each break of a wall in the prepared root is refused, naming the root
// and the file.
func TestRehearsalRegateRefusesABrokenWall(t *testing.T) {
	t.Parallel()
	publicEndpoint := regexp.MustCompile(`"http://127\.0\.0\.1:4566[^"]*"`)
	for _, tc := range []struct {
		name      string
		breakWall func(t *testing.T, root string)
		want      string
	}{
		{"one byte of the provider override flipped", func(t *testing.T, root string) {
			flipFirstByte(t, filepath.Join(root, rehearsalAWSOverride))
		}, "envs/prod/infrafactory_aws_override.tf: refused: it is not byte-identical to the override infrafactory writes"},
		{"the provider override deleted", func(t *testing.T, root string) {
			require.NoError(t, os.Remove(filepath.Join(root, rehearsalAWSOverride)))
		}, "envs/prod/infrafactory_aws_override.tf: cannot read it"},
		{"one endpoint at a public host", func(t *testing.T, root string) {
			path := filepath.Join(root, rehearsalAWSOverride)
			src, err := os.ReadFile(path)
			require.NoError(t, err)
			loc := publicEndpoint.FindIndex(src)
			require.NotNil(t, loc, "the override names no fakeaws endpoint")
			edited := append(append(append([]byte{}, src[:loc[0]]...), `"https://ec2.eu-west-1.amazonaws.com"`...), src[loc[1]:]...)
			require.NoError(t, os.WriteFile(path, edited, 0o600))
		}, "envs/prod/infrafactory_aws_override.tf: refused: it is not byte-identical to the override infrafactory writes"},
		{"one byte of the backend override flipped", func(t *testing.T, root string) {
			flipFirstByte(t, filepath.Join(root, ourOverride))
		}, "envs/prod/infrafactory_override.tf: refused: it is not byte-identical to the override infrafactory writes"},
		{"a planted zz_override.tf", func(t *testing.T, root string) {
			mustWriteFile(t, filepath.Join(root, "zz_override.tf"), "provider \"aws\" {}\n")
		}, "envs/prod/zz_override.tf: an override file is refused"},
		{"a planted .tofu file", func(t *testing.T, root string) {
			mustWriteFile(t, filepath.Join(root, "extra.tofu"), "")
		}, "envs/prod/extra.tofu: .tofu files are refused by name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, r := regatePrepared(t)
			tc.breakWall(t, filepath.Join(r.dir, regateRoot))

			err := rehearsalRegate(r, regateRoot)

			require.ErrorIs(t, err, errRehearsalRefuses)
			assert.Contains(t, err.Error(), "root "+regateRoot+": ")
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func flipFirstByte(t *testing.T, path string) {
	t.Helper()
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, src)
	src[0] ^= 1
	require.NoError(t, os.WriteFile(path, src, 0o600))
}

// The override bytes come from the record, not the file: a record whose
// port moved, over an override file left as prepare wrote it, is refused.
func TestRehearsalRegateRebuildsTheOverridesFromTheRecord(t *testing.T) {
	t.Parallel()
	repo, r := regatePrepared(t)
	record := r.record
	record.Port++
	r = rewriteRecord(t, repo, r, record)

	err := rehearsalRegate(r, regateRoot)

	require.ErrorIs(t, err, errRehearsalRefuses)
	assert.Contains(t, err.Error(), "envs/prod/infrafactory_aws_override.tf: refused: it is not byte-identical")
	ours, err := rehearsalOurOverrides(r, regateRoot)
	require.NoError(t, err)
	assert.Contains(t, string(ours[rehearsalAWSOverride]), rehearsalFakeawsURL(record.Port)+"/")
	assert.Equal(t, rehearsalBackendOverride, ours[ourOverride])
}

// Each root of the record is gated on its own: a break in the second root
// is refused naming it while the first passes, and a root the record does
// not name, or one outside its copy, is refused.
func TestRehearsalRegateIsPerRoot(t *testing.T) {
	t.Parallel()
	repo, r := regatePrepared(t)
	prod, stage := filepath.Join(r.dir, regateRoot), filepath.Join(r.dir, "work", "envs", "stage")
	require.NoError(t, rehearsalCopyTree(prod, stage))
	record := r.record
	second := record.Roots[0]
	second.Root = filepath.Join("work", "envs", "stage")
	outside := record.Roots[0]
	outside.Root = "data"
	dot := record.Roots[0]
	dot.Copy, dot.Root = ".", filepath.Join("work", "envs", "dot")
	require.NoError(t, rehearsalCopyTree(prod, filepath.Join(r.dir, dot.Root)))
	record.Roots = append(record.Roots, second, outside, dot)
	r = rewriteRecord(t, repo, r, record)
	require.NoError(t, rehearsalRegate(r, second.Root), "a clean second root")
	mustWriteFile(t, filepath.Join(stage, "zz_override.tf"), "provider \"aws\" {}\n")

	assert.NoError(t, rehearsalRegate(r, regateRoot))
	err := rehearsalRegate(r, second.Root)
	require.ErrorIs(t, err, errRehearsalRefuses)
	assert.Contains(t, err.Error(), "root work/envs/stage: ")
	assert.Contains(t, err.Error(), "envs/stage/zz_override.tf: an override file is refused")
	assert.NotContains(t, err.Error(), "envs/prod/")
	assert.EqualError(t, rehearsalRegate(r, filepath.Join("work", "envs", "qa")), "root work/envs/qa is refused: the rehearsal's record names no such root")
	_, err = rehearsalOurOverrides(r, filepath.Join("work", "envs", "qa"))
	assert.EqualError(t, err, "root work/envs/qa is refused: the rehearsal's record names no such root")
	assert.EqualError(t, rehearsalRegate(r, "data"), `root data is refused: the record must name it inside its copy "work", inside the rehearsal`)
	assert.EqualError(t, rehearsalRegate(r, dot.Root), `root work/envs/dot is refused: the record must name it inside its copy ".", inside the rehearsal`,
		"a copy of . makes the whole rehearsal the boundary")
}

// rehearsalOpen refuses a bad id, a symlinked <id> included, reports a
// missing rehearsal as no such rehearsal and creates nothing, and refuses
// a record it cannot read, still returning the dir.
func TestRehearsalOpenRefuses(t *testing.T) {
	t.Parallel()
	repo, r := regatePrepared(t)
	rehearsals := filepath.Dir(r.dir)
	require.NoError(t, os.Symlink(r.dir, filepath.Join(rehearsals, "linked")))
	mustWriteFile(t, filepath.Join(rehearsals, "plain"), "")

	for _, id := range []string{"../x", "a/b", ".", "..", "", `a\b`, "linked", "plain"} {
		_, err := rehearsalOpen(repo, id)
		assert.ErrorIs(t, err, errRehearsalBadID, id)
	}
	_, err := rehearsalOpen(repo, "other")
	assert.ErrorIs(t, err, errRehearsalMissing)

	_, bare := prepareRepo(t, nil)
	_, err = rehearsalOpen(bare, prepareID)
	assert.ErrorIs(t, err, errRehearsalMissing)
	assert.NoFileExists(t, filepath.Join(bare, ".infrafactory"))
	assert.NoDirExists(t, filepath.Join(bare, ".infrafactory"))

	for name, content := range map[string]string{"unparseable": "{", "another schema": `{"schema": "x"}`} {
		require.NoError(t, os.WriteFile(filepath.Join(r.dir, rehearsalRecordFile), []byte(content), 0o600))
		opened, err := rehearsalOpen(repo, prepareID)
		assert.ErrorIs(t, err, errRehearsalRecord, name)
		assert.Equal(t, r.dir, opened.dir, name)
	}
	record := filepath.Join(r.dir, rehearsalRecordFile)
	require.NoError(t, os.Remove(record))
	_, err = rehearsalOpen(repo, prepareID)
	assert.ErrorIs(t, err, errRehearsalRecord, "a missing record")

	// A record that is not a regular file is refused unread: a symlink to
	// a good record, and a FIFO, which a read would block on.
	good := filepath.Join(t.TempDir(), rehearsalRecordFile)
	require.NoError(t, r.record.write(filepath.Dir(good)))
	require.NoError(t, os.Symlink(good, record))
	_, err = rehearsalOpen(repo, prepareID)
	assert.ErrorIs(t, err, errRehearsalRecord, "a symlinked record")
	require.NoError(t, os.Remove(record))
	require.NoError(t, syscall.Mkfifo(record, 0o600))
	_, err = rehearsalOpen(repo, prepareID)
	assert.ErrorIs(t, err, errRehearsalRecord, "a FIFO record")
}

// A symlink above <id> is refused as a failure, never as a bad id or a
// missing rehearsal, whatever the id is named.
func TestRehearsalOpenRefusesASymlinkedRehearsalsDir(t *testing.T) {
	t.Parallel()
	_, repo := prepareRepo(t, nil)
	elsewhere := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(elsewhere, "rehearsals"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".infrafactory"), 0o700))
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(repo, ".infrafactory", "rehearsals")))

	for _, id := range []string{"rehearsals", "x"} {
		_, err := rehearsalOpen(repo, id)
		require.Error(t, err, id)
		assert.NotErrorIs(t, err, errRehearsalBadID, id)
		assert.NotErrorIs(t, err, errRehearsalMissing, id)
		assert.Contains(t, err.Error(), "a symlink would put the rehearsals outside this repository", id)
	}
}

// rehearsalOpen dials the dead proxy again and refuses, naming its
// address, when something accepts there. Not parallel: it moves
// rehearsalDeadProxy, after prepare has dialled it.
func TestRehearsalOpenRefusesALiveDeadProxy(t *testing.T) {
	repo, r := regatePrepared(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	before := rehearsalDeadProxy
	rehearsalDeadProxy = "http://" + listener.Addr().String()
	t.Cleanup(func() { rehearsalDeadProxy = before })

	opened, err := rehearsalOpen(repo, prepareID)

	require.ErrorIs(t, err, errRehearsalProxyLive)
	assert.Contains(t, err.Error(), "rehearsal "+prepareID+" is refused: ")
	assert.Contains(t, err.Error(), rehearsalDeadProxy+" accepted a connection")
	assert.Equal(t, r, opened)
}
