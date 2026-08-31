package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RunProjectMarkerFilename sits beside terraform-live.tfstate, where the
// witness it replaces used to live.
//
// ADR-0025 moved the run's project out of Terraform, so the state file no
// longer names it and `AssertProjectDeletable`'s cross-check lost its
// input. This is the replacement's first half: infrafactory writes it at
// the moment it creates the project, so it carries exactly the trust the
// state file did -- local, written by the tool during the run, never
// supplied by a caller and never by PR-supplied HCL.
//
// It is not stronger than the state file, deliberately. Someone with an
// editor can forge it exactly as they could forge tfstate. The half that
// cannot be forged locally is the API-side provenance check.
const RunProjectMarkerFilename = ".infrafactory-run-project"

// RunProjectMarker records which project a run created.
type RunProjectMarker struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
}

// WriteRunProjectMarker records the project beside the state, before any
// apply runs.
func WriteRunProjectMarker(workDir string, project RunProject) error {
	if strings.TrimSpace(project.ID) == "" {
		return fmt.Errorf("write run project marker: no project id")
	}

	payload, err := json.MarshalIndent(RunProjectMarker{ProjectID: project.ID, Name: project.Name}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run project marker: %w", err)
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create workdir for run project marker: %w", err)
	}

	// Written to a temp file and renamed, for the same reason the live
	// store does it: a truncated marker is one the guard must refuse, so
	// a half-write would strand a real project behind a guard that can
	// no longer authorise removing it.
	tmp, err := os.CreateTemp(workDir, RunProjectMarkerFilename+".*.tmp")
	if err != nil {
		return fmt.Errorf("stage run project marker: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write run project marker: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync run project marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close run project marker: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(workDir, RunProjectMarkerFilename)); err != nil {
		return fmt.Errorf("commit run project marker: %w", err)
	}

	return nil
}

// ReadRunProjectMarker returns what the run recorded. A missing or
// unreadable marker is an error, never an empty marker: the caller must
// not be able to mistake "no witness" for "no project".
func ReadRunProjectMarker(workDir string) (RunProjectMarker, error) {
	payload, err := os.ReadFile(filepath.Join(workDir, RunProjectMarkerFilename))
	if err != nil {
		return RunProjectMarker{}, fmt.Errorf("read run project marker: %w", err)
	}

	var marker RunProjectMarker
	if err := json.Unmarshal(payload, &marker); err != nil {
		return RunProjectMarker{}, fmt.Errorf("decode run project marker: %w", err)
	}
	if strings.TrimSpace(marker.ProjectID) == "" {
		return RunProjectMarker{}, fmt.Errorf("run project marker records no project id")
	}

	return marker, nil
}

// ProjectProvenance is what the API says about a project. Exists is
// false only when the API positively reported the project gone -- an
// unreachable API is an error, never an absence, because "we could not
// check" must not look like "already deleted".
type ProjectProvenance struct {
	Exists      bool
	Name        string
	Description string
}

// IsInfrafactoryRunProject reports whether the API's description of a
// project matches the stamp infrafactory puts on the ones it creates.
//
// This is the half that cannot be forged locally: defeating it means
// creating a real project shaped like a disposable run project, which is
// the kind of thing that is safe to delete.
func (p ProjectProvenance) IsInfrafactoryRunProject() bool {
	return strings.HasPrefix(p.Name, RunProjectNamePrefix) && p.Description == RunProjectDescription
}

// AssertRunProjectDeletable is the guard that stands between an
// automated teardown and someone's real infrastructure, for projects
// infrafactory created itself (ADR-0025).
//
// It replaces AssertProjectDeletable's state-derived cross-check with two
// checks that must BOTH pass:
//
//   - the marker, which says this run created THIS project. Same trust
//     level as the state file it replaces.
//   - API provenance, which says the project is an infrafactory
//     disposable one. Not locally forgeable.
//
// Neither alone is enough. The marker alone is a pure downgrade -- one
// local file swapped for another. Provenance alone is worse: it would
// authorise deleting ANY stamped project, so two runs in parallel could
// delete each other's, which the check it replaces cannot do because it
// pins to one id.
//
// The organization-default refusal is unchanged and runs first.
func AssertRunProjectDeletable(marker RunProjectMarker, targetProjectID, organizationID string, provenance ProjectProvenance) error {
	target := strings.TrimSpace(targetProjectID)
	if target == "" {
		return fmt.Errorf("%w: no project id given", ErrProtectedProject)
	}
	if orgID := strings.TrimSpace(organizationID); orgID != "" && target == orgID {
		return fmt.Errorf("%w: %s is the organization's default project", ErrProtectedProject, target)
	}

	if strings.TrimSpace(marker.ProjectID) == "" {
		return fmt.Errorf("%w: %s is not recorded in %s, so this run did not create it",
			ErrProtectedProject, target, RunProjectMarkerFilename)
	}
	if target != marker.ProjectID {
		return fmt.Errorf("%w: %s does not match the project recorded in %s (%s)",
			ErrProtectedProject, target, RunProjectMarkerFilename, marker.ProjectID)
	}

	// Already gone is the outcome asked for. Only a positive "not found"
	// reaches here; an unreachable API is an error the caller raises
	// before calling this.
	if !provenance.Exists {
		return nil
	}

	if !provenance.IsInfrafactoryRunProject() {
		return fmt.Errorf(
			"%w: %s exists but does not carry infrafactory's stamp (name %q, description %q). "+
				"The marker claims this run created it and the API disagrees -- refusing rather than guessing",
			ErrProtectedProject, target, provenance.Name, provenance.Description)
	}

	return nil
}
