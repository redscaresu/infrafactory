package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// rehearsalRecordFile is the record's name in <id>.
const rehearsalRecordFile = "rehearsal.json"

const rehearsalRecordSchema = "infrafactory.rehearsal.v1"

// errRehearsalKept is a refused prepare whose fakeaws could not be
// stopped: the rehearsal, record included, is kept so `end` can stop it.
var errRehearsalKept = errors.New("the rehearsal is kept, so its record can stop fakeaws")

// rehearsalRecord is what a later step reads to find this rehearsal's
// fakeaws and rebuild its overrides (ADR-0042 § The layout): the account
// and region it copies, the fakeaws pin, pid, port and admin token, and
// each prepared root.
type rehearsalRecord struct {
	Schema  string          `json:"schema"`
	Account string          `json:"account"`
	Region  string          `json:"region"`
	Pin     string          `json:"pin"`
	PID     int             `json:"pid"`
	Port    int             `json:"port"`
	Token   string          `json:"token"`
	Roots   []rehearsalRoot `json:"roots,omitempty"`
}

// rehearsalRoot is one prepared root, its paths relative to <id>: Copy is
// the copied tree, which is the gate's boundary (work today); Root is the
// copied root dir, the work dir prepare reports; EndpointKeys are the
// endpoints keys of its locked aws provider; AWSAliases are its provider
// "aws" aliases as the gate returns them, "" for the unaliased block.
type rehearsalRoot struct {
	Copy         string   `json:"copy"`
	Root         string   `json:"root"`
	EndpointKeys []string `json:"endpoint_keys"`
	AWSAliases   []string `json:"aws_aliases"`
}

// write writes r to <rehearsal>/rehearsal.json, 0600, through writeSynced.
func (r rehearsalRecord) write(rehearsal string) error {
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeSynced(filepath.Join(rehearsal, rehearsalRecordFile), append(encoded, '\n'))
}

// readRehearsalRecord reads <rehearsal>/rehearsal.json through
// rehearsalReadRegular, so a symlink, a FIFO or anything but a regular
// file is refused unread, and a missing one is fs.ErrNotExist. A file
// that does not parse, or holds another schema, is refused.
func readRehearsalRecord(rehearsal string) (rehearsalRecord, error) {
	path := filepath.Join(rehearsal, rehearsalRecordFile)
	raw, err := rehearsalReadRegular(path)
	if err != nil {
		return rehearsalRecord{}, fmt.Errorf("the rehearsal record %s cannot be read: %w", path, err)
	}
	var r rehearsalRecord
	if err := json.Unmarshal(raw, &r); err != nil {
		return rehearsalRecord{}, fmt.Errorf("the rehearsal record %s does not parse: %w", path, err)
	}
	if r.Schema != rehearsalRecordSchema {
		return rehearsalRecord{}, fmt.Errorf("the rehearsal record %s is refused: its schema is %q, not %q", path, r.Schema, rehearsalRecordSchema)
	}
	return r, nil
}

// walls stands the rehearsal's walls up once the second init is done
// (ADR-0042 § Provider override, § Start): each root's endpoints keys from
// its locked provider's schema, then the fakeaws binary, this rehearsal's
// fakeaws, the record, and each root's provider override. It returns the
// fakeaws URL the overrides name. The record holds each try's pid, port
// and token as soon as it is started, before its ready checks, and is
// removed once that try is stopped, so it never names a stopped try. So a
// prepare killed from there on leaves a record `end` can stop fakeaws
// with once it proves itself (rehearsalEndProof): one still starting, or
// never healthy, is left unsignalled. A start that returns a fakeaws it
// never recorded is refused. The record names the roots once every
// override is written. Any error or cancel after the
// start stops fakeaws before prepare removes the rehearsal; a stop that
// fails once the record is written, in the start or after it, is
// errRehearsalKept.
func (p rehearsePrepare) walls(ctx context.Context, rehearsal string, record rehearsalRecord) (fakeawsURL string, err error) {
	for i, root := range record.Roots {
		schema, err := p.tofu(ctx, harness.Command{Name: "tofu", Args: []string{"providers", "schema", "-json"},
			Dir: filepath.Join(rehearsal, root.Root), Env: rehearsalSecondInitEnv(rehearsal), ReplaceEnv: true})
		if err != nil {
			return "", err
		}
		if record.Roots[i].EndpointKeys, err = rehearsalEndpointKeys(schema); err != nil {
			return "", fmt.Errorf("%s: %w", root.Root, err)
		}
	}
	bin, err := p.fakeawsBin(ctx, p.runner, p.fakeawsBinPath)
	if err != nil {
		return "", err
	}
	if err := rehearsalCancelled(ctx); err != nil {
		return "", err
	}
	record.Schema, record.Pin = rehearsalRecordSchema, rehearsalFakeawsPin
	// recorded is whether the record names the try last started.
	recorded := false
	started := func(f rehearsalFakeaws) error {
		r := record
		r.Roots = nil
		r.PID, r.Port, r.Token = f.PID, f.Port, f.Token
		recorded = false
		if err := r.write(rehearsal); err != nil {
			return err
		}
		recorded = true
		return nil
	}
	stopped := func() error {
		recorded = false
		if err := os.Remove(filepath.Join(rehearsal, rehearsalRecordFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	opts := rehearsalFakeawsOpts{bin: bin, dir: filepath.Join(rehearsal, "fakeaws"), account: record.Account, region: record.Region, started: started, stopped: stopped}
	fakeaws, stop, err := p.startFakeaws(ctx, opts)
	if err != nil {
		if recorded && errors.Is(err, errRehearsalFakeawsStop) {
			err = fmt.Errorf("%w; %w", err, errRehearsalKept)
		}
		return "", err
	}
	defer func() {
		if err == nil {
			return
		}
		stopErr := stop()
		switch {
		case stopErr != nil && recorded:
			err = fmt.Errorf("%w; and fakeaws's %w; %w", err, stopErr, errRehearsalKept)
		case stopErr != nil:
			err = fmt.Errorf("%w; and fakeaws's %w; %s", err, stopErr, fakeaws.orphaned())
		}
	}()
	if !recorded {
		return "", errors.New("fakeaws is refused: it started, but its record was never written")
	}
	record.PID, record.Port, record.Token = fakeaws.PID, fakeaws.Port, fakeaws.Token
	fakeawsURL = rehearsalFakeawsURL(record.Port)
	for _, root := range record.Roots {
		if err := writeSynced(filepath.Join(rehearsal, root.Root, rehearsalAWSOverride), record.providerOverride(root)); err != nil {
			return "", err
		}
	}
	if err := record.write(rehearsal); err != nil {
		return "", err
	}
	if err := rehearsalTightenModes(rehearsal); err != nil {
		return "", err
	}
	return fakeawsURL, rehearsalCancelled(ctx)
}
