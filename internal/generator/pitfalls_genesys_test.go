package generator

import "testing"

// TestExtractResourceFromDetail_Genesys is the regression test for the
// S118-uncovered bug: the resourceNameRe regex only matched the three
// IaaS cloud prefixes (scaleway|google|aws). genesyscloud_* resources
// returned an empty string, which made ExtractDescriptivePitfall
// return nil, which meant zero pitfalls were ever auto-learned from
// genesys failures — the dynamic loop oscillated for 5 iterations
// and gave up with repair_budget_exhausted on genesys-architect-flow
// during sustain sweep 1.
func TestExtractResourceFromDetail_Genesys(t *testing.T) {
	cases := []struct {
		name   string
		detail string
		want   string
	}{
		{
			name: "architect flow filepath",
			detail: `exit status 1 | stderr: ╷
│ Error: could not open flows/ivr-with-lookup.yaml: no such file or directory
│   with genesyscloud_flow.ivr,
│   on main.tf line 24, in resource "genesyscloud_flow" "ivr":`,
			want: "genesyscloud_flow",
		},
		{
			name: "user invalid argument",
			detail: `Error: Unsupported argument
  on main.tf line 14, in resource "genesyscloud_user" "agent_one":
  An argument named "roles" is not expected here.`,
			want: "genesyscloud_user",
		},
		{
			name: "routing queue invalid argument",
			detail: `Error: Unsupported argument
  on main.tf line 32, in resource "genesyscloud_routing_queue" "support":
  An argument named "skills" is not expected here.`,
			want: "genesyscloud_routing_queue",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractResourceFromDetail(c.detail)
			if got != c.want {
				t.Errorf("ExtractResourceFromDetail = %q, want %q", got, c.want)
			}
		})
	}
}
