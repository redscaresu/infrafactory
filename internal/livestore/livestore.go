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
	RunID     string    `json:"run_id,omitempty"`
	ProjectID string    `json:"project_id"`
	Region    string    `json:"region,omitempty"`
	Zone      string    `json:"zone,omitempty"`
	Address   string    `json:"address,omitempty"`
	Image     string    `json:"image,omitempty"`
	Tag       string    `json:"tag,omitempty"`
	State     State     `json:"state"`
	WorkDir   string    `json:"work_dir,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
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
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("id is required")
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

// FilesystemStore keeps one JSON file per deployment under Root.
type FilesystemStore struct {
	Root string
}

func NewFilesystemStore(root string) *FilesystemStore {
	if root == "" {
		root = DefaultRoot
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
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("id is required")
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

	if err := os.WriteFile(s.path(d.ID), payload, 0o644); err != nil {
		return fmt.Errorf("write deployment: %w", err)
	}

	return nil
}

// Get reads one deployment by id.
func (s *FilesystemStore) Get(id string) (Deployment, error) {
	payload, err := os.ReadFile(s.path(id))
	if err != nil {
		return Deployment{}, fmt.Errorf("read deployment %s: %w", id, err)
	}

	var d Deployment
	if err := json.Unmarshal(payload, &d); err != nil {
		return Deployment{}, fmt.Errorf("decode deployment %s: %w", id, err)
	}

	return d, nil
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
			unreadable = append(unreadable, err)
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
func (s *FilesystemStore) MarkReleased(id string) error {
	d, err := s.Get(id)
	if err != nil {
		return err
	}
	d.State = StateReleased
	return s.write(d)
}

func (s *FilesystemStore) path(id string) string {
	return filepath.Join(s.Root, id+".json")
}
