package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/feedback"
)

// RealScalewayAPIBase is the only API a sweep will talk to.
const RealScalewayAPIBase = "https://api.scaleway.com"

// ProjectResourceType is the Terraform type that carries the run's
// self-managed project (ADR-0010).
const ProjectResourceType = "scaleway_account_project"

var ErrOrphanSweepFailed = errors.New("orphan sweep failed")

// ScalewayOrphanSweep answers "did this run leave anything billable
// behind?" against the real API.
//
// Before this existed, `destruction: no_orphans` was evaluated against
// mockway state even for Layer 3 runs -- so a destroy that half-worked
// reported clean while real resources kept billing. Nothing ever asked
// Scaleway what survived.
//
// The self-managed project (ADR-0010) is what makes the check cheap and
// exact: the run's entire blast radius is one project, so "did we leak?"
// reduces to "is that project gone, and did everything actually live
// inside it?"
type ScalewayOrphanSweep struct {
	apiBase string
	doer    func(*http.Request) (*http.Response, error)
}

func NewScalewayOrphanSweep(timeout time.Duration) *ScalewayOrphanSweep {
	client := &http.Client{Timeout: timeout}
	return &ScalewayOrphanSweep{apiBase: RealScalewayAPIBase, doer: client.Do}
}

// NewScalewayOrphanSweepWithDoer is the test seam. Mirrors how
// RealProbeHarness injects its dialer/resolver/client.
func NewScalewayOrphanSweepWithDoer(apiBase string, doer func(*http.Request) (*http.Response, error)) *ScalewayOrphanSweep {
	return &ScalewayOrphanSweep{apiBase: apiBase, doer: doer}
}

type OrphanSweepResult struct {
	ProjectID string
	Failures  []feedback.Failure
}

func (r *OrphanSweepResult) Clean() bool { return r != nil && len(r.Failures) == 0 }

// Run reports every reason to believe the run leaked.
//
// Every uncertain outcome is a failure, never a skip. A sweep that
// cannot reach the API is exactly when you most want a red result --
// "we could not check" and "nothing leaked" must never look alike.
func (s *ScalewayOrphanSweep) Run(ctx context.Context, workDir string, secretKey string) (*OrphanSweepResult, error) {
	state, err := loadLiveTerraformState(filepath.Join(workDir, LiveStateFilename))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOrphanSweepFailed, err)
	}

	projectID := runProjectID(state)
	if projectID == "" {
		return nil, fmt.Errorf("%w: live state has no %s resource, so the run's blast radius cannot be determined (ADR-0010 requires one)",
			ErrOrphanSweepFailed, ProjectResourceType)
	}

	result := &OrphanSweepResult{ProjectID: projectID}
	result.Failures = append(result.Failures, strayResourceFailures(state, projectID)...)

	remaining, err := s.projectExists(ctx, projectID, secretKey)
	if err != nil {
		result.Failures = append(result.Failures, feedback.Failure{
			Layer:   "sandbox_deploy",
			Stage:   "orphan_sweep",
			Check:   "project_deleted",
			Command: "GET " + s.apiBase + "/account/v3/projects/" + projectID,
			Detail: fmt.Sprintf("could not verify that project %s was destroyed: %v. Treating as a leak: an unverifiable sweep must not look like a clean one. Re-check with `tofu destroy -state=%s` in %s once connectivity is restored.",
				projectID, err, LiveStateFilename, workDir),
		})
		return result, nil
	}
	if remaining {
		result.Failures = append(result.Failures, feedback.Failure{
			Layer:   "sandbox_deploy",
			Stage:   "orphan_sweep",
			Check:   "project_deleted",
			Command: "GET " + s.apiBase + "/account/v3/projects/" + projectID,
			Detail: fmt.Sprintf("project %s still exists after destroy — it is billable until removed. Tear it down with `tofu destroy -state=%s` in %s, then confirm in the Scaleway console.",
				projectID, LiveStateFilename, workDir),
		})
	}
	return result, nil
}

// runProjectID extracts the id of the project this run created.
func runProjectID(state terraformState) string {
	for _, resource := range state.Resources {
		if resource.Type != ProjectResourceType {
			continue
		}
		for _, instance := range resource.Instances {
			if id, ok := instance.Attributes["id"].(string); ok && strings.TrimSpace(id) != "" {
				return id
			}
		}
	}
	return ""
}

// strayResourceFailures catches resources that were created OUTSIDE the
// run's project.
//
// The project-scoped API check alone is not sufficient. A generated
// resource that omits project_id lands in the organization's default
// project; destroying the run project then succeeds, the sweep sees a
// 404, and reports clean while the stray resource bills indefinitely.
// Terraform state is the only place that records where things actually
// went, so check it directly.
func strayResourceFailures(state terraformState, projectID string) []feedback.Failure {
	strays := make([]string, 0)
	for _, resource := range state.Resources {
		if resource.Type == ProjectResourceType {
			continue
		}
		for _, instance := range resource.Instances {
			raw, present := instance.Attributes["project_id"]
			if !present {
				// No project_id attribute at all: the type is not
				// project-scoped (or the provider does not expose it).
				// Not evidence of a stray.
				continue
			}
			got, _ := raw.(string)
			if strings.TrimSpace(got) == "" || got == projectID {
				continue
			}
			id, _ := instance.Attributes["id"].(string)
			strays = append(strays, fmt.Sprintf("%s (%s) in project %s", resource.Type, id, got))
		}
	}
	if len(strays) == 0 {
		return nil
	}
	sort.Strings(strays)
	return []feedback.Failure{{
		Layer:   "sandbox_deploy",
		Stage:   "orphan_sweep",
		Check:   "resources_outside_run_project",
		Command: "inspect " + LiveStateFilename,
		Detail: fmt.Sprintf("%d resource(s) were created outside the run project %s: %s. Destroying the run project will not remove them and a project-scoped sweep cannot see them — they must carry project_id = scaleway_account_project.<name>.id",
			len(strays), projectID, strings.Join(strays, "; ")),
	}}
}

// projectExists reports whether the project is still present. 404 means
// destroyed; anything unexpected is an error, never a silent false.
func (s *ScalewayOrphanSweep) projectExists(ctx context.Context, projectID, secretKey string) (bool, error) {
	url := s.apiBase + "/account/v3/projects/" + projectID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := s.doer(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return false, nil
	case http.StatusOK:
		return true, nil
	default:
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return false, fmt.Errorf("unexpected status %d from %s (body: %v)", resp.StatusCode, url, body)
	}
}

// ErrProtectedProject is returned when a destroy path is pointed at a
// project the run did not create.
var ErrProtectedProject = errors.New("refusing to delete a project this run did not create")

// AssertProjectDeletable is the guard that stands between an automated
// teardown and someone's real infrastructure.
//
// A destroy path may only ever remove the project recorded in the state
// file it was handed. Anything else -- a hand-typed id, a stale state
// file, the organization's default project -- is refused. The default
// project is refused unconditionally because its id equals the
// organization id and deleting it would take the whole account's
// contents with it.
func AssertProjectDeletable(stateProjectID, targetProjectID, organizationID string) error {
	target := strings.TrimSpace(targetProjectID)
	if target == "" {
		return fmt.Errorf("%w: no project id given", ErrProtectedProject)
	}
	if orgID := strings.TrimSpace(organizationID); orgID != "" && target == orgID {
		return fmt.Errorf("%w: %s is the organization's default project", ErrProtectedProject, target)
	}
	if strings.TrimSpace(stateProjectID) == "" {
		return fmt.Errorf("%w: %s is not recorded in %s, so this run did not create it", ErrProtectedProject, target, LiveStateFilename)
	}
	if target != stateProjectID {
		return fmt.Errorf("%w: %s does not match the project recorded in %s (%s)", ErrProtectedProject, target, LiveStateFilename, stateProjectID)
	}
	return nil
}

// RunProjectIDFromState is the exported accessor reap uses to learn what
// a given run is allowed to destroy.
func RunProjectIDFromState(workDir string) (string, error) {
	state, err := loadLiveTerraformState(filepath.Join(workDir, LiveStateFilename))
	if err != nil {
		return "", err
	}
	return runProjectID(state), nil
}
