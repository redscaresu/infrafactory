package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ScalewayInstancePowerOff stops every Instance in a run's project so the
// destroy that follows can succeed.
//
// # Why a teardown has to do this at all
//
// A Scaleway private NIC is deletable only while its server is POWERED OFF.
// Terraform destroys in reverse dependency order, so the NIC -- which
// references the server -- goes first, with the server still running, and the
// API refuses:
//
//	precondition failed: Can't delete a private network interface attached to a server
//
// Measured on real Scaleway on 2026-09-09 and 2026-09-10: four applies, four
// failed destroys. The auto-destroy failed on the same expression each time, so
// every one of those runs left billable infrastructure up and needed a human
// with shell access and cloud credentials to unpick it by hand -- always the
// same way, `scw instance server stop` followed by the delete.
//
// # Why it is not fixed in the HCL
//
// It was tried. ADR-0029 refused the standalone
// `scaleway_instance_private_nic` and prescribed the provider's inline
// `private_network` block instead, on the reasoning that a server's own delete
// powers it off first and takes its NICs with it. That reasoning was wrong: the
// provider detaches the NIC as its own API call either way, and the inline
// shape failed teardown identically. NO HCL SHAPE expresses a destroyable
// private-network attachment on a running instance, and when no configuration
// works the configuration is not where the fix goes.
//
// # Before the destroy, not after it
//
// The auto-created security-group purge is remediation: it reacts to a failure
// nobody could have predicted from the plan. This is a PRECONDITION -- it is
// known in advance, for every stack that declares compute. Running it first
// costs a few list calls and a poweroff on servers that are about to be deleted
// anyway; running it as a retry would make every compute teardown pay for a
// failed destroy first.
//
// Best-effort, like the purge. The authoritative "did we leak?" answer belongs
// to ScalewayOrphanSweep, which fails closed. A transient list error here must
// not become a failed teardown -- the destroy is still going to run, and it
// still reports for itself.
type ScalewayInstancePowerOff struct {
	apiBase string
	doer    func(*http.Request) (*http.Response, error)
	// pollDelay is a field so tests do not sleep. Production keeps the
	// real delay; a fake doer sets it to zero.
	pollDelay time.Duration
	// pollAttempts bounds the wait for a server to reach `stopped`. A
	// poweroff is not instant and a destroy started too early hits the
	// very precondition this exists to clear.
	pollAttempts int
}

func NewScalewayInstancePowerOff(timeout time.Duration) *ScalewayInstancePowerOff {
	client := &http.Client{Timeout: timeout}
	return &ScalewayInstancePowerOff{
		apiBase:      RealScalewayAPIBase,
		doer:         client.Do,
		pollDelay:    3 * time.Second,
		pollAttempts: 40, // 40 x 3s = 120s, sized to a real Scaleway poweroff
	}
}

// NewScalewayInstancePowerOffWithDoer is the test seam. No sleeping.
func NewScalewayInstancePowerOffWithDoer(apiBase string, doer func(*http.Request) (*http.Response, error)) *ScalewayInstancePowerOff {
	return &ScalewayInstancePowerOff{apiBase: apiBase, doer: doer, pollDelay: 0, pollAttempts: 40}
}

// PowerOffNotStoppedMarker appears in a result entry for a server that
// was asked to stop and did not get there.
//
// A shared constant rather than each side matching prose, because the
// two sides disagreeing is a FALSE GREEN: the caller renders the result
// as a passing stage that says instances were powered off, and a server
// still running is the case where the destroy is about to fail. The
// stage must be able to tell the two apart without guessing.
const PowerOffNotStoppedMarker = "did NOT reach stopped"

type instanceServer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Project string `json:"project"`
	State   string `json:"state"`
}

// stoppedStates are the states from which a NIC delete succeeds. Scaleway
// reports two kinds of off: `stopped` releases the resources, `stopped in
// place` keeps them allocated. Both are powered off, which is what the NIC
// precondition asks about.
var stoppedStates = map[string]bool{
	"stopped":          true,
	"stopped in place": true,
}

// Run powers off every server in projectID and returns what it stopped.
//
// Does no ownership checking of its own -- projectID must already have passed
// AssertProjectDeletable, exactly as for the purge. cli.destroySandbox applies
// that guard.
func (p *ScalewayInstancePowerOff) Run(ctx context.Context, projectID, secretKey string) ([]string, error) {
	if projectID == "" {
		return nil, fmt.Errorf("poweroff requires a project id")
	}

	var stopped []string
	for _, zone := range InstanceZones {
		servers, err := p.listServers(ctx, zone, projectID, secretKey)
		if err != nil {
			continue // lenient by design -- see the type comment
		}
		for _, s := range servers {
			if stoppedStates[s.State] {
				continue
			}
			if err := p.poweroff(ctx, zone, s.ID, secretKey); err != nil {
				continue
			}
			if err := p.waitStopped(ctx, zone, s.ID, secretKey); err != nil {
				// Reported rather than swallowed: a server that never
				// reached `stopped` is the case where the destroy is
				// about to fail, and the operator should see why.
				stopped = append(stopped, fmt.Sprintf("server %s (%s) in %s %s: %v", s.ID, s.Name, zone, PowerOffNotStoppedMarker, err))
				continue
			}
			stopped = append(stopped, fmt.Sprintf("server %s (%s) in %s", s.ID, s.Name, zone))
		}
	}
	return stopped, nil
}

// listServers returns only servers that really are inside projectID.
//
// The API is asked to filter and the response is checked, for the same reason
// the purge does it: this drives a state change on real infrastructure, and the
// credential can see every project in the organization. Trusting a query
// parameter to be the only thing between it and another project's servers is a
// hope, not a guarantee.
func (p *ScalewayInstancePowerOff) listServers(ctx context.Context, zone, projectID, secretKey string) ([]instanceServer, error) {
	url := fmt.Sprintf("%s/instance/v1/zones/%s/servers?project=%s&per_page=100", p.apiBase, zone, projectID)
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
		return nil, fmt.Errorf("list servers in %s: status %d", zone, resp.StatusCode)
	}

	var body struct {
		Servers []instanceServer `json:"servers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	scoped := make([]instanceServer, 0, len(body.Servers))
	for _, s := range body.Servers {
		if s.Project == projectID {
			scoped = append(scoped, s)
		}
	}
	return scoped, nil
}

func (p *ScalewayInstancePowerOff) poweroff(ctx context.Context, zone, id, secretKey string) error {
	url := fmt.Sprintf("%s/instance/v1/zones/%s/servers/%s/action", p.apiBase, zone, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(`{"action":"poweroff"}`))
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.doer(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 202 is the documented answer -- the action returns a task. A server
	// already stopping answers 400 with `invalid state`, which is not a
	// failure for our purposes: waitStopped decides.
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("poweroff %s: status %d", id, resp.StatusCode)
	}
	return nil
}

// waitStopped blocks until the server reports a powered-off state.
//
// The wait is the point. `poweroff` returns as soon as the task is accepted,
// and a destroy started at that moment hits the same precondition -- which is
// exactly how the manual recovery failed the first time it was tried, before
// the `-w` flag was added to the stop.
func (p *ScalewayInstancePowerOff) waitStopped(ctx context.Context, zone, id, secretKey string) error {
	for attempt := 0; attempt < p.pollAttempts; attempt++ {
		state, err := p.serverState(ctx, zone, id, secretKey)
		if err == nil && stoppedStates[state] {
			return nil
		}
		if p.pollDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.pollDelay):
			}
		}
	}
	return fmt.Errorf("still not stopped after %d checks", p.pollAttempts)
}

func (p *ScalewayInstancePowerOff) serverState(ctx context.Context, zone, id, secretKey string) (string, error) {
	url := fmt.Sprintf("%s/instance/v1/zones/%s/servers/%s", p.apiBase, zone, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := p.doer(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get server %s: status %d", id, resp.StatusCode)
	}

	var body struct {
		Server instanceServer `json:"server"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Server.State, nil
}
