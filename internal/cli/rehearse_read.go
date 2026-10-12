package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/spf13/cobra"
)

const rehearseReadSchema = "infrafactory.rehearse.read.v1"

// rehearseReadFile is the recorded read's name in --out.
const rehearseReadFile = "read.json"

// rehearseRead is what `rehearse read` runs with. A zero field takes its
// default.
type rehearseRead struct {
	// doer sends every request, each through its service's wall:
	// newAWSHTTPClient() by default.
	doer aws.HTTPClient
	// wd is the current directory: os.Getwd by default.
	wd string
}

// rehearseReadFlags are the flags, as given. The account and region come
// from here only, never from infrafactory.yaml or the environment.
type rehearseReadFlags struct {
	account, principal, region, credentials, readSet, out string
}

// newRehearseReadCmd is `rehearse read` (ADR-0041): one read step. It
// reads no infrafactory.yaml and sets no persistent pre-run: the group's
// makes a missing required flag a usage error.
func newRehearseReadCmd(r rehearseRead) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read an AWS account, read-only, through the reader table into a recorded read under .infrafactory/",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return rehearseUsage(cmd, err)
			}
			return nil
		},
		RunE: r.run,
	}
	cmd.Flags().String("account", "", "the AWS account id the key must belong to (12 digits)")
	cmd.Flags().String("principal", "", "the ARN the key must be, matched exactly; unchecked when not given")
	cmd.Flags().String("region", "", "the AWS region to read")
	cmd.Flags().String("credentials", "", "the reader key file, mode 0600: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and an optional AWS_SESSION_TOKEN")
	cmd.Flags().String("read-set", "", "a JSON array of ADR-0041 read-set entries")
	cmd.Flags().String("out", "", "a directory under .infrafactory/, outside runs and live, to write "+rehearseReadFile+" into")
	for _, name := range []string{"account", "region", "credentials", "read-set", "out"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func (r rehearseRead) run(cmd *cobra.Command, _ []string) error {
	var f rehearseReadFlags
	f.account, _ = cmd.Flags().GetString("account")
	f.principal, _ = cmd.Flags().GetString("principal")
	f.region, _ = cmd.Flags().GetString("region")
	f.credentials, _ = cmd.Flags().GetString("credentials")
	f.readSet, _ = cmd.Flags().GetString("read-set")
	f.out, _ = cmd.Flags().GetString("out")
	if !harness.ValidAWSAccountID(f.account) {
		return rehearseUsage(cmd, fmt.Errorf("--account %q is not a 12-digit AWS account id", f.account))
	}
	if !harness.ValidAWSRegion(f.region) {
		return rehearseUsage(cmd, fmt.Errorf("--region %q is not an AWS region name", f.region))
	}
	mode, err := outputModeFromCommand(cmd)
	if err != nil {
		return err
	}
	if r.wd == "" {
		if r.wd, err = os.Getwd(); err != nil {
			return &CLIError{Op: cmd.CommandPath(), Code: errorCodeCommandFailed, Err: err}
		}
	}
	if r.doer == nil {
		r.doer = newAWSHTTPClient()
	}

	result := rehearseReadResult{Schema: rehearseReadSchema, Status: CommandStatusSuccess}
	read, path, err := r.read(cmd.Context(), f)
	if err != nil {
		result = rehearseReadResult{Schema: rehearseReadSchema, Status: CommandStatusFailed, Refusal: err.Error()}
	} else {
		result.Read, result.Exchanges = path, len(read.Exchanges)
	}
	if writeErr := result.write(cmd, mode); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return &CLIError{Op: cmd.CommandPath(), Code: errorCodeCommandFailed, Err: err}
	}
	return nil
}

// read makes --out before anything is read, then reads the read set, the
// key (held in this process only) and the account, and writes the
// recorded read into --out. It returns the read and its path.
func (r rehearseRead) read(ctx context.Context, f rehearseReadFlags) (harness.AWSRecordedRead, string, error) {
	repo, err := rehearsalRepo(r.wd)
	if err != nil {
		return harness.AWSRecordedRead{}, "", err
	}
	out, err := rehearseReadOutDir(repo, r.abs(f.out))
	if err != nil {
		return harness.AWSRecordedRead{}, "", err
	}
	readSet, err := readRehearseReadSet(r.abs(f.readSet))
	if err != nil {
		return harness.AWSRecordedRead{}, "", err
	}
	cfg, err := harness.AWSReaderConfig(r.abs(f.credentials), f.region)
	if err != nil {
		return harness.AWSRecordedRead{}, "", err
	}
	read, err := harness.ReadAWSAccount(ctx, harness.AWSReadInput{
		Config: cfg, Account: f.account, Principal: f.principal, ReadSet: readSet, Doer: r.doer,
		Identity: harness.VerifyAWSReadIdentity, Preflight: harness.PreflightAWSReadKey,
	})
	if err != nil {
		return harness.AWSRecordedRead{}, "", err
	}
	// Bodies are XML: unescaped, the recording stays legible to check.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(read); err != nil {
		return harness.AWSRecordedRead{}, "", fmt.Errorf("encode the recorded read: %w", err)
	}
	path := filepath.Join(out, rehearseReadFile)
	return read, path, writeSynced(path, buf.Bytes())
}

func (r rehearseRead) abs(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(r.wd, path)
}

// rehearseReadOutDir makes out, resolved, a 0700 dir and returns it. It
// refuses, making nothing, an out that does not resolve under
// <repo>/.infrafactory, or that lies in the run or live store, which the
// UI serves. Those are compared case-folded: on a case-insensitive file
// system .infrafactory/RUNS is the run store. An existing dir open to
// group or other is refused and never chmodded.
func rehearseReadOutDir(repo, out string) (string, error) {
	resolved, err := rehearsalResolveExisting(out)
	if err != nil {
		return "", fmt.Errorf("--out %s is refused: it does not resolve: %w", out, err)
	}
	root := filepath.Join(repo, ".infrafactory")
	if resolved == root || !rehearsalUnder(resolved, root) {
		return "", fmt.Errorf("--out %s is refused: it resolves to %s, which is not under %s", out, resolved, root)
	}
	for _, served := range []string{runstore.DefaultRoot, livestore.DefaultRoot} {
		if rehearsalUnder(strings.ToLower(resolved), strings.ToLower(filepath.Join(repo, served))) {
			return "", fmt.Errorf("--out %s is refused: it lies in %s, which the UI serves", out, served)
		}
	}
	if err := os.MkdirAll(resolved, 0o700); err != nil {
		return "", fmt.Errorf("--out %s: %w", out, err)
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("--out %s: %w", out, err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("--out %s is refused: %s is %v, not a directory of mode 0700, and is never chmodded", out, resolved, info.Mode())
	}
	return resolved, nil
}

// rehearsalResolveExisting is path with its deepest existing ancestor,
// path itself included, resolved and the rest joined back on, so no
// symlink on the way can carry it elsewhere. A dangling symlink there does
// not resolve.
func rehearsalResolveExisting(path string) (string, error) {
	dir, rest := filepath.Clean(path), ""
	for {
		_, err := os.Lstat(dir)
		if err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if !errors.Is(err, fs.ErrNotExist) || parent == dir {
			return "", err
		}
		rest, dir = filepath.Join(filepath.Base(dir), rest), parent
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, rest), nil
}

// readRehearseReadSet is the file's read-set entries: one JSON array,
// decoded strictly. The reader validates each entry.
func readRehearseReadSet(path string) ([]harness.AWSReadEntry, error) {
	// Only a regular file: opening a FIFO would wait for a writer.
	if info, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("--read-set %s: %w", path, err)
	} else if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("--read-set %s is refused: it is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("--read-set %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var entries []harness.AWSReadEntry
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("--read-set %s is not a JSON array of read-set entries: %w", path, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("--read-set %s carries data after the array", path)
	}
	return entries, nil
}

// rehearseReadResult is what read reports: where the recorded read went
// and how many exchanges it holds, or the refusal.
type rehearseReadResult struct {
	Schema    string        `json:"schema"`
	Status    CommandStatus `json:"status"`
	Read      string        `json:"read,omitempty"`
	Exchanges int           `json:"exchanges,omitempty"`
	Refusal   string        `json:"refusal,omitempty"`
}

func (r rehearseReadResult) write(cmd *cobra.Command, mode OutputMode) error {
	out := cmd.OutOrStdout()
	if mode == OutputModeJSON {
		encoded, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return fmt.Errorf("encode read result: %w", err)
		}
		_, err = fmt.Fprintf(out, "%s\n", encoded)
		return err
	}
	_, _ = fmt.Fprintf(out, "Command: rehearse read\nStatus: %s\n", r.Status)
	if r.Refusal != "" {
		_, err := fmt.Fprintf(out, "Refused: %s\n", r.Refusal)
		return err
	}
	_, err := fmt.Fprintf(out, "Recorded read: %s\nExchanges: %d\n", r.Read, r.Exchanges)
	return err
}
