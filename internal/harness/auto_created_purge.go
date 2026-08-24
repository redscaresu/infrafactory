package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// InstanceZones is every zone Scaleway will place an Instance in.
//
// The purge checks all of them rather than the run's configured zone
// because a generated stack is free to name any zone it likes, and a
// purge that looked only where it expected the run to be would leave the
// project undeletable exactly when the generator surprised us.
//
// Enumerated against the live API on 2026-08-24 rather than from
// memory -- the first version of this list omitted it-mil-1, which is a
// real Instance zone. To re-verify, list security groups per candidate
// zone and keep the ones that answer 200:
//
//	GET /instance/v1/zones/<zone>/security_groups?project=<id>
//
// A zone missing from this list does not leak silently: the retried
// destroy still fails and ScalewayOrphanSweep still fails closed. It
// degrades to a loud failure, not a quiet leak.
var InstanceZones = []string{
	"fr-par-1", "fr-par-2", "fr-par-3",
	"nl-ams-1", "nl-ams-2", "nl-ams-3",
	"pl-waw-1", "pl-waw-2", "pl-waw-3",
	"it-mil-1",
}

// ScalewayAutoCreatedPurge removes resources the API creates by itself
// inside a run's project -- things Terraform never created and therefore
// never destroys.
//
// This exists because ADR-0010's disposable project stops being
// disposable the moment a scenario declares an Instance. Creating the
// first Instance in a fresh project causes Scaleway to auto-create a
// "Default security group" in that project. It is not in the plan, not
// in the state file, and not Terraform's to remove -- but the project
// cannot be deleted while it is there, so `tofu destroy` ends with
//
//	precondition failed: resource is still in use, all resources are not deleted
//
// and the run leaks a project on every single execution.
//
// Deliberately best-effort. The authoritative "did we leak?" answer
// belongs to ScalewayOrphanSweep, which fails closed. If this purge
// misses something the retried destroy still fails and the sweep still
// reports -- so being lenient here costs nothing, while being strict
// would turn a transient list error into a failed teardown.
type ScalewayAutoCreatedPurge struct {
	apiBase string
	doer    func(*http.Request) (*http.Response, error)
}

func NewScalewayAutoCreatedPurge(timeout time.Duration) *ScalewayAutoCreatedPurge {
	client := &http.Client{Timeout: timeout}
	return &ScalewayAutoCreatedPurge{apiBase: RealScalewayAPIBase, doer: client.Do}
}

// NewScalewayAutoCreatedPurgeWithDoer is the test seam.
func NewScalewayAutoCreatedPurgeWithDoer(apiBase string, doer func(*http.Request) (*http.Response, error)) *ScalewayAutoCreatedPurge {
	return &ScalewayAutoCreatedPurge{apiBase: apiBase, doer: doer}
}

type securityGroup struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Project        string `json:"project"`
	ProjectDefault bool   `json:"project_default"`
}

// Run deletes auto-created resources in projectID and reports what it
// removed.
//
// This type does no ownership checking of its own and will delete what
// it is pointed at, so projectID must already have passed
// AssertProjectDeletable. cli.destroySandbox is the only caller and
// applies that guard itself rather than trusting its own callers to.
func (p *ScalewayAutoCreatedPurge) Run(ctx context.Context, projectID, secretKey string) ([]string, error) {
	if projectID == "" {
		return nil, fmt.Errorf("purge requires a project id")
	}

	var removed []string
	for _, zone := range InstanceZones {
		groups, err := p.listDefaultSecurityGroups(ctx, zone, projectID, secretKey)
		if err != nil {
			// Lenient by design -- see the type comment.
			continue
		}
		for _, g := range groups {
			if err := p.deleteSecurityGroup(ctx, zone, g.ID, secretKey); err != nil {
				continue
			}
			removed = append(removed, fmt.Sprintf("security_group %s (%s) in %s", g.ID, g.Name, zone))
		}
	}
	return removed, nil
}

// listDefaultSecurityGroups returns only groups that are BOTH inside
// projectID and marked by Scaleway as that project's default.
//
// Two independent conditions, deliberately. A group the run's own HCL
// created is Terraform's to destroy, and deleting it here would hide a
// real destroy bug -- that is what project_default screens out. The
// membership check is defence in depth for the other direction: this
// code deletes things, and trusting a query parameter to be the only
// thing standing between it and another project's resources is not a
// guarantee, it is a hope. The credential can see every project in the
// organization, so a changed filter name or an unfiltered page would be
// enough. The API is asked to filter and the response is checked.
func (p *ScalewayAutoCreatedPurge) listDefaultSecurityGroups(ctx context.Context, zone, projectID, secretKey string) ([]securityGroup, error) {
	url := fmt.Sprintf("%s/instance/v1/zones/%s/security_groups?project=%s&per_page=100", p.apiBase, zone, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := p.doer(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d listing security groups in %s", resp.StatusCode, zone)
	}

	var body struct {
		SecurityGroups []securityGroup `json:"security_groups"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	var defaults []securityGroup
	for _, g := range body.SecurityGroups {
		if g.ProjectDefault && g.Project == projectID {
			defaults = append(defaults, g)
		}
	}
	return defaults, nil
}

func (p *ScalewayAutoCreatedPurge) deleteSecurityGroup(ctx context.Context, zone, id, secretKey string) error {
	url := fmt.Sprintf("%s/instance/v1/zones/%s/security_groups/%s", p.apiBase, zone, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := p.doer(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d deleting security group %s", resp.StatusCode, id)
	}
	return nil
}
