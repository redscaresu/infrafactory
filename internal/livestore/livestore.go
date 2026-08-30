// Package livestore records infrastructure that deliberately outlives the
// run which created it.
//
// Everything else in this project is built on the opposite promise: a run
// applies to a disposable project, destroys it, and sweeps the account
// (ADR-0010, ADR-0023). Live deployments are the scoped exception, and this
// store is the only thing that says which ones are legitimate. Anything
// running in the account that is not recorded here is a leak, not a feature.
//
// Two rules make that safe, and both are enforced here rather than by
// convention:
//
//   - An expiry is mandatory. There is no value of ExpiresAt meaning
//     "forever", and Put refuses a record without one.
//   - Unreadable is expired. A record that will not parse, or that parses
//     without an expiry or a project id, is reported as expired so the
//     reaper takes it down. "We cannot tell" must never be the state that
//     keeps billing.
package livestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultRoot sits beside .infrafactory/runs so both stores share a fate:
// wiping the working directory loses the record, which is why the reaper
// reconciles against the API rather than trusting this file alone.
const DefaultRoot = ".infrafactory/live"

const DeploymentSchemaVersion = "infrafactory.live.deployment.v1"

// LiveStateFilename mirrors harness.LiveStateFilename. Duplicated rather
// than imported to keep this package free of a dependency on the harness;
// TestLiveStateFilenameMatchesHarness pins them together.
const LiveStateFilename = "terraform-live.tfstate"

// State is where a deployment is in its lifecycle. It is deliberately not
// a proxy for what the cloud believes -- only the API can answer that.
type State string

const (
	// StateLive means the deployment was applied and not yet torn down.
	StateLive State = "live"
	// StateReleased means teardown ran and reported success. The record is
	// kept rather than deleted so the reaper can prove it acted.
	StateReleased State = "released"
)

// Deployment is one live service. ProjectID is the load-bearing field: it
// is the blast radius, and the only handle the reaper has to destroy what
// this record describes.
type Deployment struct {
	Schema    string    `json:"schema,omitempty"`
	ID        string    `json:"id"`
	Scenario  string    `json:"scenario"`
	ProjectID string    `json:"project_id"`
	Address   string    `json:"address,omitempty"`
	Image     string    `json:"image,omitempty"`
	Tag       string    `json:"tag,omitempty"`
	State     State     `json:"state"`
	WorkDir   string    `json:"work_dir,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`

	// SweepVerificationFailed records that an orphan sweep for this
	// deployment has failed at least once. It is sticky: once a sweep has
	// reported strays, no later pass may release the record on the
	// strength of an empty state, because destroy has erased the state the
	// strays would be recomputed from.
	SweepVerificationFailed bool `json:"sweep_verification_failed,omitempty"`

	// Undecodable marks a record the store could not parse. It is never
	// persisted: it exists so an unreadable file still reaches the
	// reaper as a deployment rather than only as a log line. ADR-0024
	// rule 3 says unreadable means expired, and a record that never
	// entered the reapable set did not honour that.
	Undecodable bool `json:"-"`
}

// Expired reports whether the deployment's TTL has run out. A zero
// ExpiresAt is expired, not immortal: see the package comment.
func (d Deployment) Expired(now time.Time) bool {
	if d.ExpiresAt.IsZero() {
		return true
	}
	return !now.Before(d.ExpiresAt)
}

// Reapable reports whether the reaper should act on this record. Released
// deployments are already gone; everything else that has expired is fair
// game, including records this package could not fully understand.
func (d Deployment) Reapable(now time.Time) bool {
	if d.State == StateReleased {
		return false
	}
	return d.Expired(now)
}

// TimeToLive is how long remains before expiry, floored at zero so callers
// never render a negative duration as if it meant something.
func (d Deployment) TimeToLive(now time.Time) time.Duration {
	if d.Expired(now) {
		return 0
	}
	return d.ExpiresAt.Sub(now)
}

// Validate enforces what a record must carry to be actionable later. It is
// the fail-closed gate: a deployment that cannot be reaped must not be
// written in the first place.
func (d Deployment) Validate() error {
	if err := validateID(d.ID); err != nil {
		return err
	}
	if strings.TrimSpace(d.Scenario) == "" {
		return fmt.Errorf("scenario is required")
	}
	// Without a project id there is nothing to destroy and nothing to
	// scope a sweep to, so the record would describe a leak we could not
	// clean up.
	if strings.TrimSpace(d.ProjectID) == "" {
		return fmt.Errorf("project_id is required: a deployment that cannot be located cannot be reaped")
	}
	if d.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if d.ExpiresAt.IsZero() {
		return fmt.Errorf("expires_at is required: live deployments have no unbounded form")
	}
	if !d.ExpiresAt.After(d.CreatedAt) {
		return fmt.Errorf("expires_at (%s) must be after created_at (%s)",
			d.ExpiresAt.Format(time.RFC3339), d.CreatedAt.Format(time.RFC3339))
	}
	return nil
}

// validateID refuses anything that would escape the store when joined
// into a path. Ids are derived from scenario names, which the schema does
// not constrain to a safe charset, and `live teardown <id>` takes one
// straight from the command line.
func validateID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return fmt.Errorf("id is required")
	}
	if trimmed != id {
		return fmt.Errorf("id %q has leading or trailing whitespace", id)
	}
	if strings.ContainsRune(id, os.PathSeparator) || strings.Contains(id, "/") {
		return fmt.Errorf("id %q contains a path separator", id)
	}
	if id == "." || id == ".." || strings.Contains(id, "..") {
		return fmt.Errorf("id %q contains a parent-directory reference", id)
	}
	return nil
}

// FilesystemStore keeps one JSON file per deployment under Root.
type FilesystemStore struct {
	Root string
}

// NewFilesystemStore resolves root to an absolute path. A relative root
// would make the store mean different things to different callers: a
// scheduled reaper with another working directory would find nothing,
// report "nothing has expired", exit 0, and leave deployments billing.
func NewFilesystemStore(root string) *FilesystemStore {
	if root == "" {
		root = DefaultRoot
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return &FilesystemStore{Root: root}
}

// Put writes a deployment record, refusing anything Validate rejects.
func (s *FilesystemStore) Put(d Deployment) error {
	if err := d.Validate(); err != nil {
		return fmt.Errorf("invalid deployment: %w", err)
	}
	return s.write(d)
}

// write persists a record without revalidating it. Only MarkReleased uses
// this: a record that fails Validate is still reapable, so once its
// resources are gone that outcome must be recordable. Routing the release
// through Put would refuse the write, and the reaper would destroy the
// same already-destroyed deployment on every pass.
func (s *FilesystemStore) write(d Deployment) error {
	if err := validateID(d.ID); err != nil {
		return err
	}
	if d.Schema == "" {
		d.Schema = DeploymentSchemaVersion
	}
	if d.State == "" {
		d.State = StateLive
	}

	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return fmt.Errorf("create live store: %w", err)
	}

	payload, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("encode deployment: %w", err)
	}

	// Written to a temp file and renamed. os.WriteFile truncates first,
	// so an interrupt or a full disk mid-write leaves exactly the
	// half-parsed record this package says must never exist -- and a
	// concurrent reader sees it. Rename is atomic within a directory.
	tmp, err := os.CreateTemp(s.Root, "."+d.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("stage deployment write: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write deployment: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync deployment: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close deployment: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod deployment: %w", err)
	}
	if err := os.Rename(tmpName, s.path(d.ID)); err != nil {
		return fmt.Errorf("commit deployment: %w", err)
	}

	return nil
}

// Get reads one deployment by id.
func (s *FilesystemStore) Get(id string) (Deployment, error) {
	// Validated on the READ path as well: `live teardown <id>` passes a
	// command-line argument straight here, and a traversing id would
	// otherwise read an arbitrary file and, if it happened to decode,
	// feed a chosen WorkDir and ProjectID into teardown.
	if err := validateID(id); err != nil {
		return Deployment{}, err
	}

	payload, err := os.ReadFile(s.path(id))
	if err != nil {
		return Deployment{}, fmt.Errorf("read deployment %s: %w", id, err)
	}

	var d Deployment
	if err := json.Unmarshal(payload, &d); err != nil {
		return Deployment{}, fmt.Errorf("decode deployment %s: %w", id, err)
	}

	if d.WorkDir != "" && !filepath.IsAbs(d.WorkDir) {
		d.WorkDir = s.resolveLegacyWorkDir(d.WorkDir)
	}

	return d, nil
}

// resolveLegacyWorkDir makes sense of a relative WorkDir written before
// roots were absolute. Such a record is unreclaimable from any other
// working directory -- reported as "may still be running" on every pass,
// forever -- so it is worth resolving rather than failing.
//
// Two candidates are tried and the one that actually holds the state
// wins, rather than assuming a layout: the process's working directory
// (where it was written from, if that has not changed) and the store
// root's grandparent (the repo root, for the default
// `.infrafactory/live` placement). If neither has the state, the
// absolute form of the original is returned so the error names a real
// path.
func (s *FilesystemStore) resolveLegacyWorkDir(workDir string) string {
	candidates := []string{workDir}
	if grandparent := filepath.Dir(filepath.Dir(s.Root)); grandparent != "" {
		candidates = append(candidates, filepath.Join(grandparent, workDir))
	}

	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if fileExists(filepath.Join(abs, LiveStateFilename)) {
			return abs
		}
	}

	if abs, err := filepath.Abs(workDir); err == nil {
		return abs
	}
	return workDir
}

// List returns every record, newest first. Records that fail to decode are
// reported through the second return value rather than dropped: a file the
// store cannot read may still be real infrastructure, and silently
// omitting it would turn a leak into an empty list.
func (s *FilesystemStore) List() ([]Deployment, []error, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read live store: %w", err)
	}

	deployments := make([]Deployment, 0, len(entries))
	var unreadable []error

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		d, err := s.Get(id)
		if err != nil {
			// Reported AND returned as a deployment. A record the store
			// cannot parse may describe running infrastructure; leaving
			// it out of the set meant the reaper never saw it, which is
			// not what ADR-0024 rule 3 promises.
			unreadable = append(unreadable, err)
			deployments = append(deployments, Deployment{ID: id, Undecodable: true})
			continue
		}
		deployments = append(deployments, d)
	}

	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].CreatedAt.After(deployments[j].CreatedAt)
	})

	return deployments, unreadable, nil
}

// Reapable returns the deployments whose TTL has run out and which have
// not already been released.
func (s *FilesystemStore) Reapable(now time.Time) ([]Deployment, []error, error) {
	all, unreadable, err := s.List()
	if err != nil {
		return nil, nil, err
	}

	expired := make([]Deployment, 0, len(all))
	for _, d := range all {
		if d.Reapable(now) {
			expired = append(expired, d)
		}
	}

	return expired, unreadable, nil
}

// MarkReleased records that teardown succeeded. The record is retained so
// the reaper's own history is auditable.
// MarkReleased records that teardown succeeded.
//
// A record that will not decode can still be released: its resources may
// have been destroyed by hand, and if the only way to clear it were to
// decode it first, it would sit in the reapable set forever failing every
// pass. The replacement keeps the id and nothing else, because nothing
// else was recoverable.
func (s *FilesystemStore) MarkReleased(id string) error {
	// Before any side effect. The fallback path below reads and writes
	// s.path(id); without this an id that Get rejects still reached
	// os.ReadFile/os.WriteFile and copied a file to an operator-chosen
	// location outside the store before the write was finally refused.
	if err := validateID(id); err != nil {
		return err
	}

	d, err := s.Get(id)
	if err != nil {
		if !fileExists(s.path(id)) {
			return err
		}
		// The unparseable bytes are preserved alongside, not replaced.
		// A record truncated mid-write often still contains a readable
		// project id -- the one thing an operator would need to finish
		// the job by hand -- and overwriting it in place would destroy
		// that while marking the deployment "released", so no reaper
		// would ever look at it again.
		if raw, readErr := os.ReadFile(s.path(id)); readErr == nil {
			_ = os.WriteFile(s.path(id)+".unreadable", raw, 0o644)
		}
		return s.write(Deployment{ID: id, State: StateReleased})
	}
	d.State = StateReleased
	return s.write(d)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *FilesystemStore) path(id string) string {
	return filepath.Join(s.Root, id+".json")
}
