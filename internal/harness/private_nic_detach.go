package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ScalewayPrivateNICDetach deletes a run project's private NICs through
// the Instance v1 API, before `tofu destroy` runs.
//
// # The defect this works around
//
// Provider 2.81.0 deletes a private NIC through Instance **v2alpha1**, and
// that endpoint refuses:
//
//	DELETE /instance/v2alpha1/zones/fr-par-1/private-network-interfaces/{id}
//	412  precondition_failed  resource_not_usable
//	     "Can't delete a private network interface attached to a server"
//
// A private NIC is BY DEFINITION attached to a server, so on this endpoint the
// precondition can never be satisfied and the provider can never destroy one.
// The v1 route has no such precondition:
//
//	DELETE /instance/v1/zones/fr-par-1/servers/{server}/private_nics/{id}
//	204
//
// Both were issued against the SAME NIC on the SAME server within seconds of
// each other on 2026-09-10. With the NIC removed via v1, `tofu destroy` then
// completed: `Destroy complete! Resources: 3 destroyed.`
//
// # What this replaces, and the three wrong answers before it
//
// The failure was chased for two days as a power-state problem, because the
// manual recovery -- `scw instance server stop`, then delete -- worked. It
// worked because the `scw` CLI uses **v1**. Stopping the server was incidental
// and did nothing; the endpoint was the whole difference.
//
// That mistake produced three retracted claims, each measured against
// something other than the thing itself:
//
//  1. ADR-0029: the inline `private_network` block destroys cleanly. It does
//     not -- same endpoint, same 412.
//  2. ADR-0030: powering the server off first fixes teardown. It does not; a
//     verified-stopped server still got the 412.
//  3. Eventual consistency: retry and it clears. It does not -- three retries
//     over sixty seconds, identical failure.
//
// What finally settled it was issuing both DELETEs by hand and comparing the
// status codes. Two curl calls, after two days.
//
// # Why it is safe to do behind Terraform's back
//
// The NIC is the provider's to own, and this removes it first -- so the
// subsequent server delete finds nothing to remove and proceeds. Scoped to one
// project, which must already have passed AssertProjectDeletable, and reported
// so a teardown never silently deletes anything.
type ScalewayPrivateNICDetach struct {
	apiBase string
	doer    func(*http.Request) (*http.Response, error)
}

func NewScalewayPrivateNICDetach(timeout time.Duration) *ScalewayPrivateNICDetach {
	client := &http.Client{Timeout: timeout}
	return &ScalewayPrivateNICDetach{apiBase: RealScalewayAPIBase, doer: client.Do}
}

// NewScalewayPrivateNICDetachAt points the detach at a chosen API base.
//
// Layer 2 needs this. mockway reproduces the v2alpha1 refusal faithfully
// (mockway#28), which is the mock doing its job -- and it means the mock
// destroy hits exactly the same wall as the real one, so it needs the
// same v1 detach first. Same code, different base URL.
func NewScalewayPrivateNICDetachAt(apiBase string, timeout time.Duration) *ScalewayPrivateNICDetach {
	client := &http.Client{Timeout: timeout}
	return &ScalewayPrivateNICDetach{apiBase: apiBase, doer: client.Do}
}

// NewScalewayPrivateNICDetachWithDoer is the test seam.
func NewScalewayPrivateNICDetachWithDoer(apiBase string, doer func(*http.Request) (*http.Response, error)) *ScalewayPrivateNICDetach {
	return &ScalewayPrivateNICDetach{apiBase: apiBase, doer: doer}
}

// Run deletes every private NIC on every server in projectID and reports
// what it removed.
//
// Best-effort, like the auto-created purge: the destroy runs regardless and
// reports for itself, and ScalewayOrphanSweep is what fails closed. A list
// error here must not become a failed teardown.
func (d *ScalewayPrivateNICDetach) Run(ctx context.Context, projectID, secretKey string) ([]string, error) {
	if projectID == "" {
		return nil, fmt.Errorf("private nic detach requires a project id")
	}

	var removed []string
	for _, zone := range InstanceZones {
		servers, err := d.listServers(ctx, zone, projectID, secretKey)
		if err != nil {
			// Reported, not swallowed. A silent `continue` made an auth
			// failure or a changed route look identical to "this project
			// has no servers" -- and the caller then logs "no private
			// NICs to remove" and proceeds into a destroy that is about
			// to fail. That is the failure mode this whole arc was
			// about; it does not get to reappear in the fix for it.
			removed = append(removed, fmt.Sprintf("could NOT be deleted: listing servers in %s failed: %v", zone, err))
			continue
		}
		for _, s := range servers {
			nics, err := d.listNICs(ctx, zone, s.ID, secretKey)
			if err != nil {
				removed = append(removed, fmt.Sprintf("could NOT be deleted: listing NICs for server %s in %s failed: %v", s.ID, zone, err))
				continue
			}
			for _, nic := range nics {
				if err := d.deleteNIC(ctx, zone, s.ID, nic.ID, secretKey); err != nil {
					removed = append(removed, fmt.Sprintf("private nic %s on server %s in %s could NOT be deleted: %v", nic.ID, s.ID, zone, err))
					continue
				}
				removed = append(removed, fmt.Sprintf("private nic %s on server %s in %s", nic.ID, s.ID, zone))
			}
		}
	}
	return removed, nil
}

type nicServer struct {
	ID      string `json:"id"`
	Project string `json:"project"`
}

type privateNIC struct {
	ID string `json:"id"`
}

// listNICs asks the dedicated route rather than reading `private_nics`
// off the server object.
//
// Real Scaleway embeds that field; mockway does not, so a detach that
// depended on it silently found nothing at Layer 2 -- server located,
// project matched, zero NICs, no stage, no error. Asking the endpoint
// that exists in both is one request more and one assumption fewer.
func (d *ScalewayPrivateNICDetach) listNICs(ctx context.Context, zone, serverID, secretKey string) ([]privateNIC, error) {
	url := fmt.Sprintf("%s/instance/v1/zones/%s/servers/%s/private_nics", d.apiBase, zone, serverID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := d.doer(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list private nics for %s: status %d", serverID, resp.StatusCode)
	}

	var body struct {
		PrivateNICs []privateNIC `json:"private_nics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.PrivateNICs, nil
}

// listServers checks the project on every server it returns, for the same
// reason the purge does: the credential can see the whole organization, and a
// query parameter is not on its own allowed to be what keeps this off another
// project's infrastructure.
func (d *ScalewayPrivateNICDetach) listServers(ctx context.Context, zone, projectID, secretKey string) ([]nicServer, error) {
	url := fmt.Sprintf("%s/instance/v1/zones/%s/servers?project=%s&per_page=100", d.apiBase, zone, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := d.doer(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list servers in %s: status %d", zone, resp.StatusCode)
	}

	var body struct {
		Servers []nicServer `json:"servers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	scoped := make([]nicServer, 0, len(body.Servers))
	for _, s := range body.Servers {
		if s.Project == projectID {
			scoped = append(scoped, s)
		}
	}
	return scoped, nil
}

// deleteNIC uses the v1 route deliberately. v2alpha1 is the one the provider
// calls and the one that cannot succeed -- swapping this for the newer-looking
// endpoint would reintroduce the whole defect.
func (d *ScalewayPrivateNICDetach) deleteNIC(ctx context.Context, zone, serverID, nicID, secretKey string) error {
	url := fmt.Sprintf("%s/instance/v1/zones/%s/servers/%s/private_nics/%s", d.apiBase, zone, serverID, nicID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := d.doer(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 404 means somebody already removed it, which is the outcome we wanted.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
