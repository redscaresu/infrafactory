package scenario

import (
	"path/filepath"
	"testing"
)

// TestLoadAWSScenarios validates that the training scenarios shipped
// in S43-T13 (scenarios/training/aws-iam.yaml + aws-s3.yaml) parse
// against the schema, declare cloud:aws, and surface their
// AWSResourceAnchors. Mirror of the gcp scenario loader test.
//
// Scenario YAML files live in the repo's scenarios/training/ tree.
// We resolve from this test file's directory upward to find the
// schema and the scenarios.
func TestLoadAWSScenarios(t *testing.T) {
	repo := repoRootForAWSTest(t)
	schemaPath := filepath.Join(repo, "scenario.schema.json")

	cases := []struct {
		path                string
		expectedAnchors     []string
		expectedCloud       string
		expectedDescription string
	}{
		{
			path:            filepath.Join(repo, "scenarios", "training", "aws-iam.yaml"),
			expectedCloud:   "aws",
			expectedAnchors: []string{"aws_iam_role", "aws_iam_policy", "aws_iam_role_policy_attachment"},
		},
		{
			path:            filepath.Join(repo, "scenarios", "training", "aws-s3.yaml"),
			expectedCloud:   "aws",
			expectedAnchors: []string{"aws_s3_bucket", "aws_s3_bucket_versioning", "aws_s3_bucket_server_side_encryption_configuration"},
		},
		{
			path:          filepath.Join(repo, "scenarios", "training", "aws-vpc-network.yaml"),
			expectedCloud: "aws",
			expectedAnchors: []string{
				"aws_vpc", "aws_subnet", "aws_internet_gateway",
				"aws_route_table", "aws_route", "aws_route_table_association",
				"aws_security_group",
			},
		},
		{
			path:          filepath.Join(repo, "scenarios", "training", "aws-instance.yaml"),
			expectedCloud: "aws",
			expectedAnchors: []string{
				"aws_vpc", "aws_subnet", "aws_security_group", "aws_instance",
			},
		},
		{
			path:          filepath.Join(repo, "scenarios", "training", "aws-rds.yaml"),
			expectedCloud: "aws",
			expectedAnchors: []string{
				"aws_vpc", "aws_subnet", "aws_db_subnet_group",
				"aws_db_parameter_group", "aws_db_instance",
			},
		},
		{
			path:          filepath.Join(repo, "scenarios", "training", "aws-dynamodb.yaml"),
			expectedCloud: "aws",
			expectedAnchors: []string{"aws_dynamodb_table"},
		},
		{
			path:          filepath.Join(repo, "scenarios", "training", "aws-eks.yaml"),
			expectedCloud: "aws",
			expectedAnchors: []string{
				"aws_iam_role", "aws_vpc", "aws_subnet",
				"aws_eks_cluster", "aws_eks_node_group", "aws_eks_addon",
			},
		},
		{
			path:          filepath.Join(repo, "scenarios", "training", "aws-sqs.yaml"),
			expectedCloud: "aws",
			expectedAnchors: []string{"aws_sqs_queue"},
		},
	}
	for _, c := range cases {
		t.Run(filepath.Base(c.path), func(t *testing.T) {
			sc, err := LoadWithSchema(c.path, schemaPath)
			if err != nil {
				t.Fatalf("LoadWithSchema: %v", err)
			}
			if sc.Cloud != c.expectedCloud {
				t.Errorf("cloud: got %q want %q", sc.Cloud, c.expectedCloud)
			}
			if len(sc.AWSResourceAnchors) != len(c.expectedAnchors) {
				t.Fatalf("AWSResourceAnchors count: got %d want %d (%v)",
					len(sc.AWSResourceAnchors), len(c.expectedAnchors), sc.AWSResourceAnchors)
			}
			for i, want := range c.expectedAnchors {
				if sc.AWSResourceAnchors[i] != want {
					t.Errorf("anchor[%d]: got %q want %q", i, sc.AWSResourceAnchors[i], want)
				}
			}
		})
	}
}

// TestLoadAWSScenariosCoverageAuditCanReadAnchors asserts a fresh aws
// scenario added by a future service-bundle PR (S44+) will surface its
// AWSResourceAnchors via the Scenario struct so S48-T7's
// TestFullCoverageAudit can credit it. Locks the schema → struct
// round-trip.
func TestLoadAWSScenariosCoverageAuditCanReadAnchors(t *testing.T) {
	repo := repoRootForAWSTest(t)
	schemaPath := filepath.Join(repo, "scenario.schema.json")
	scPath := filepath.Join(repo, "scenarios", "training", "aws-iam.yaml")

	sc, err := LoadWithSchema(scPath, schemaPath)
	if err != nil {
		t.Fatalf("LoadWithSchema: %v", err)
	}
	// The audit (in fakeaws/internal/audit/audit_test.go) probes the
	// scenario YAML directly via regex; the Go-side struct path is the
	// alternative entry point for callers that don't want to re-parse
	// the file. Both must surface the same anchors.
	if !sliceContainsAWSTest(sc.AWSResourceAnchors, "aws_iam_role") {
		t.Errorf("AWSResourceAnchors missing aws_iam_role: %v", sc.AWSResourceAnchors)
	}
}

// repoRootForAWSTest walks up from this test file to find the
// scenario.schema.json marker (mirror of the audit_test.go helper in
// fakeaws — keeps the test independent of working directory).
func repoRootForAWSTest(t *testing.T) string {
	t.Helper()
	wd, _ := filepath.Abs(".")
	for i := 0; i < 6; i++ {
		if _, err := filepath.Glob(filepath.Join(wd, "scenario.schema.json")); err == nil {
			matches, _ := filepath.Glob(filepath.Join(wd, "scenario.schema.json"))
			if len(matches) > 0 {
				return wd
			}
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	t.Fatalf("could not locate infrafactory repo root from %s", wd)
	return ""
}

func sliceContainsAWSTest(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
