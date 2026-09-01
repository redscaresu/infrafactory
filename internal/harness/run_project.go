package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// RunProjectNamePrefix marks a project as one infrafactory created for a
// run. It is the provenance stamp ADR-0025 relies on: a project without
// it was not created here, so nothing this tool does may destroy it.
//
// S165 only *applies* the stamp. Verifying it at teardown is S166, which
// replaces AssertProjectDeletable's state-derived cross-check and needs
// review on its own.
const RunProjectNamePrefix = "if-run-"

// RunProjectDescription is the second half of the stamp. A name can be
// typed by hand; a name plus this exact description is a much weaker
// coincidence.
const RunProjectDescription = "infrafactory Layer 3 run project (ADR-0025). Disposable: created before apply, destroyed after."

// runProjectNameUnsafe matches anything that must not reach a project
// name. Names are derived from scenario names, which the schema already
// constrains, but the stamp is a security marker and is built here rather
// than trusted from a caller.
var runProjectNameUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// runProjectNameHyphenRun collapses repeated separators. Unsafe
// characters become hyphens, so `a b__c` would otherwise produce runs.
var runProjectNameHyphenRun = regexp.MustCompile(`-{2,}`)

// maxRunProjectNameLength keeps the generated name inside Scaleway's
// limit with room for the prefix and stamp.
const maxRunProjectNameLength = 60

// RunProject is a project infrafactory created for one Layer 3 run.
type RunProject struct {
	ID   string
	Name string
}

// ScalewayRunProject creates and deletes the disposable project a Layer 3
// run applies into.
//
// ADR-0025: the project is created BEFORE tofu runs and passed to the
// provider as SCW_DEFAULT_PROJECT_ID, so resources that carry no
// project_id of their own -- scaleway_instance_private_nic has no such
// attribute at all -- land in the run's own project rather than the
// shared fallback. Creating it in the HCL made that impossible, because
// the id did not exist when the provider's environment was built.
type ScalewayRunProject struct {
	apiBase string
	doer    func(*http.Request) (*http.Response, error)
}

func NewScalewayRunProject(timeout time.Duration) *ScalewayRunProject {
	client := &http.Client{Timeout: timeout}
	return &ScalewayRunProject{apiBase: RealScalewayAPIBase, doer: client.Do}
}

// NewScalewayRunProjectWithDoer is the test seam, matching
// NewScalewayOrphanSweepWithDoer and NewScalewayAutoCreatedPurgeWithDoer.
func NewScalewayRunProjectWithDoer(apiBase string, doer func(*http.Request) (*http.Response, error)) *ScalewayRunProject {
	return &ScalewayRunProject{apiBase: apiBase, doer: doer}
}

// RunProjectName builds the stamped, safe project name for a scenario.
// Exported so callers can predict the name without creating anything.
func RunProjectName(scenario, stamp string) string {
	clean := func(s string) string {
		s = runProjectNameUnsafe.ReplaceAllString(strings.ToLower(s), "-")
		return strings.Trim(runProjectNameHyphenRun.ReplaceAllString(s, "-"), "-")
	}

	name := RunProjectNamePrefix + clean(scenario)
	if s := clean(stamp); s != "" {
		name += "-" + s
	}
	if len(name) > maxRunProjectNameLength {
		name = strings.TrimRight(name[:maxRunProjectNameLength], "-")
	}
	return name
}

// Create makes the run's project and returns it.
//
// The caller passes the organization because a project has to be created
// in one, and because sandboxCommandEnv already refuses to run without
// SCW_DEFAULT_ORGANIZATION_ID for exactly that reason.
func (p *ScalewayRunProject) Create(ctx context.Context, secretKey, organizationID, scenario, stamp string) (RunProject, error) {
	if strings.TrimSpace(secretKey) == "" {
		return RunProject{}, fmt.Errorf("create run project: no secret key")
	}
	if strings.TrimSpace(organizationID) == "" {
		return RunProject{}, fmt.Errorf("create run project: no organization id")
	}

	name := RunProjectName(scenario, stamp)
	payload, err := json.Marshal(map[string]string{
		"name":            name,
		"organization_id": organizationID,
		"description":     RunProjectDescription,
	})
	if err != nil {
		return RunProject{}, fmt.Errorf("encode create-project request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.apiBase+"/account/v3/projects", bytes.NewReader(payload))
	if err != nil {
		return RunProject{}, fmt.Errorf("build create-project request: %w", err)
	}
	req.Header.Set("X-Auth-Token", secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.doer(req)
	if err != nil {
		return RunProject{}, fmt.Errorf("create project %q: %w", name, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return RunProject{}, fmt.Errorf("create project %q: http %d: %s",
			name, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return RunProject{}, fmt.Errorf("decode create-project response: %w", err)
	}
	// Fail closed: a create that reports success without an id leaves a
	// project nothing can find, which is worse than a failed create.
	if strings.TrimSpace(decoded.ID) == "" {
		return RunProject{}, fmt.Errorf("create project %q: response carried no id, so the project cannot be tracked or destroyed", name)
	}

	return RunProject{ID: decoded.ID, Name: decoded.Name}, nil
}

// Describe reports what the API says about a project, for the provenance
// half of AssertRunProjectDeletable.
//
// A 404 is a positive answer -- the project is gone -- and returns
// Exists false with no error. Any other failure is an error, because an
// unreachable API must never be mistaken for "already deleted": that is
// the same fail-closed rule the orphan sweep applies, and the reason a
// teardown refuses rather than proceeds when it cannot check.
func (p *ScalewayRunProject) Describe(ctx context.Context, secretKey, projectID string) (ProjectProvenance, error) {
	if strings.TrimSpace(secretKey) == "" {
		return ProjectProvenance{}, fmt.Errorf("describe run project: no secret key")
	}
	if strings.TrimSpace(projectID) == "" {
		return ProjectProvenance{}, fmt.Errorf("describe run project: no project id")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.apiBase+"/account/v3/projects/"+projectID, nil)
	if err != nil {
		return ProjectProvenance{}, fmt.Errorf("build describe-project request: %w", err)
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := p.doer(req)
	if err != nil {
		return ProjectProvenance{}, fmt.Errorf("describe project %s: %w", projectID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ProjectProvenance{Exists: false}, nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ProjectProvenance{}, fmt.Errorf("describe project %s: http %d: %s",
			projectID, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ProjectProvenance{}, fmt.Errorf("decode describe-project response: %w", err)
	}

	return ProjectProvenance{Exists: true, Name: decoded.Name, Description: decoded.Description}, nil
}

// Delete removes the run's project once its resources are gone.
//
// `tofu destroy` no longer removes it -- the project is not a Terraform
// resource under ADR-0025 -- so teardown calls this after the destroy, in
// the same place destroySandbox already purges what the API auto-created.
//
// A 404 is success: the project is gone, which is the outcome asked for.
func (p *ScalewayRunProject) Delete(ctx context.Context, secretKey, projectID string) error {
	if strings.TrimSpace(secretKey) == "" {
		return fmt.Errorf("delete run project: no secret key")
	}
	if strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("delete run project: no project id")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		p.apiBase+"/account/v3/projects/"+projectID, nil)
	if err != nil {
		return fmt.Errorf("build delete-project request: %w", err)
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := p.doer(req)
	if err != nil {
		return fmt.Errorf("delete project %s: %w", projectID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("delete project %s: http %d: %s",
			projectID, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

// ListedProject is one project the API knows about, with the fields the
// provenance stamp is read from.
type ListedProject struct {
	ID          string
	Name        string
	Description string
}

// Provenance re-reads the stamp from a listed project, so a caller uses
// one rule for "is this ours" whether it listed the project or described
// it.
func (l ListedProject) Provenance() ProjectProvenance {
	return ProjectProvenance{Exists: true, Name: l.Name, Description: l.Description}
}

// maxProjectListPages bounds the pagination walk.
//
// Not a tuning knob: it is the difference between "this account has more
// projects than expected" and an unbounded loop against a paid API if it
// ever returns a page that does not shrink. At 100 per page it allows
// 5,000 projects, far past anything this tool creates.
const maxProjectListPages = 50

const projectListPageSize = 100

// List returns every project in the organization.
//
// It does NOT filter to infrafactory's own. The caller decides what is
// theirs, using the same `IsInfrafactoryRunProject` stamp that guards
// teardown, so there is one definition of ownership rather than one here
// and another there.
//
// Reading the whole organization is deliberate and is the only way to
// answer the question reconciliation asks -- *what is running that
// nothing knows about?* A list filtered by the API could only return
// projects matching something we already knew to ask for, which is the
// assumption being checked.
func (p *ScalewayRunProject) List(ctx context.Context, secretKey, organizationID string) ([]ListedProject, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("list projects: no secret key")
	}
	if strings.TrimSpace(organizationID) == "" {
		return nil, fmt.Errorf("list projects: no organization id")
	}

	var out []ListedProject
	for page := 1; page <= maxProjectListPages; page++ {
		listed, err := p.listPage(ctx, secretKey, organizationID, page)
		if err != nil {
			return nil, err
		}
		out = append(out, listed...)
		if len(listed) < projectListPageSize {
			// A short page is the last page. Stopping on an EMPTY page
			// instead would spend one extra request per call, every
			// call, against an API that charges for nothing else here
			// but is rate limited.
			return out, nil
		}
	}
	// Fail rather than return a truncated list. A caller comparing this
	// against the live store would read missing projects as "nothing
	// unaccounted for", which is the precise falsehood reconciliation
	// exists to prevent.
	return nil, fmt.Errorf("list projects: more than %d pages in organization %s; refusing to report a partial estate",
		maxProjectListPages, organizationID)
}

func (p *ScalewayRunProject) listPage(ctx context.Context, secretKey, organizationID string, page int) ([]ListedProject, error) {
	endpoint := fmt.Sprintf("%s/account/v3/projects?organization_id=%s&page=%d&page_size=%d",
		p.apiBase, url.QueryEscape(organizationID), page, projectListPageSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build list-projects request: %w", err)
	}
	req.Header.Set("X-Auth-Token", secretKey)

	resp, err := p.doer(req)
	if err != nil {
		return nil, fmt.Errorf("list projects (page %d): %w", page, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("list projects (page %d): http %d: %s",
			page, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Projects []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode list-projects response: %w", err)
	}

	out := make([]ListedProject, 0, len(decoded.Projects))
	for _, pr := range decoded.Projects {
		out = append(out, ListedProject{ID: pr.ID, Name: pr.Name, Description: pr.Description})
	}
	return out, nil
}
