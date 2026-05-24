package e2e

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_AWSFullStack runs the aws-full-stack training scenario
// composition against fakeaws: VPC + 2 subnets, IAM cluster + node
// roles, EKS cluster + node group, RDS DB subnet + parameter group
// + Postgres DB instance, S3 bucket, and a Secrets Manager secret +
// initial version. Mirrors TestE2E_FullStackParis (Scaleway) and
// TestE2E_GCPFullStack (GCP) — same lifecycle contract.
//
// Composition exercised: VPC + 2 subnets + 2 IAM roles + EKS
// cluster + EKS node group + S3 bucket (with SSE) + RDS subnet
// group + parameter group + DB instance + Secrets Manager secret +
// secret version. VPC / IAM / EKS run against fakeaws after the
// M51 envelope rewrite + M57 per-resource field parity; RDS was
// fully wired in M61 (DbiResourceId stability, service-specific
// 404 codes, DeleteDBInstance envelope, user-supplied field
// persistence); Secrets Manager was fully wired in M62 (ARN-or-
// name SecretId resolution, DescribeSecret field parity including
// VersionIdsToStages + epoch timestamps, GetResourcePolicy /
// ListSecretVersionIds handlers). S3 runs against SeaweedFS
// (M59) — the third-party S3 backend chosen after Adobe S3Mock +
// Garage both failed evaluation.
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1 and requires `tofu` on PATH.
func TestE2E_AWSFullStack(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakeaws(t)
	// SeaweedFS is the M59-chosen third-party S3 backend (Apache 2.0,
	// implements the full bucket-management surface terraform-
	// provider-aws's Read flow needs — Adobe S3Mock and Garage both
	// failed evaluation; LocalStack community is gone). See CONCEPT.md
	// "Third-Party Mock Integration" for the decision trail.
	s3 := StartSeaweedFS(t)

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := repoScenariosPath(t, "aws-full-stack.yaml")

	WriteConfigMultiCloud(t, configPath, "http://127.0.0.1:1", "", mock.URL, s3.URL, outputRoot)

	files := awsFullStackFiles(mock.URL, s3.URL)

	// Apply with --no-destroy so we can introspect state before the
	// final destroy stage tears everything down.
	noDestroy := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath, "--no-destroy"},
		GeneratorFiles: files,
	})
	if noDestroy.Err != nil {
		t.Fatalf("run --no-destroy failed: %v\nstdout:\n%s\nstderr:\n%s\nfakeaws log: %s",
			noDestroy.Err, noDestroy.Stdout, noDestroy.Stderr, mock.LogPath())
	}
	for _, want := range []string{"Status: success", "run/terminal_reason: pass (target_reached)"} {
		if !strings.Contains(noDestroy.Stdout, want) {
			t.Fatalf("expected first-run stdout to contain %q, got:\n%s", want, noDestroy.Stdout)
		}
	}

	// Verify every service block has at least the expected resource
	// count after apply. fakeaws's /mock/state uses per-service maps
	// of slice-valued collections (ec2.vpcs, iam.roles, rds.instances,
	// etc.) — same shape as fakegcp.
	state := mock.FetchState(t)
	// VPC / IAM / EKS / RDS / Secrets Manager all asserted against
	// fakeaws state after M61 + M62. The collection paths reflect
	// gatherRDSStateReal + gatherSecretsManagerStateReal in fakeaws.
	for _, exp := range []struct {
		root       string
		collection string
		minCount   int
	}{
		{root: "ec2", collection: "vpcs", minCount: 1},
		{root: "ec2", collection: "subnets", minCount: 2},
		{root: "iam", collection: "roles", minCount: 2},
		{root: "eks", collection: "clusters", minCount: 1},
		{root: "eks", collection: "node_groups", minCount: 1},
		{root: "rds", collection: "db_instances", minCount: 1},
		{root: "rds", collection: "db_subnet_groups", minCount: 1},
		{root: "secretsmanager", collection: "secrets", minCount: 1},
	} {
		got := awsStateItemCount(state, exp.root, exp.collection)
		if got < exp.minCount {
			t.Errorf("expected at least %d %s/%s after apply, got %d",
				exp.minCount, exp.root, exp.collection, got)
		}
	}
	// S3 bucket asserted against SeaweedFS via HEAD (M59).
	// SeaweedFS's ListAllMyBuckets returns empty in anonymous mode
	// even when buckets exist (they're owned by no one without
	// proper IAM setup), so HEAD-by-name is the reliable existence
	// check. The bucket name is pinned in storage.tf, so this is
	// deterministic. The cloudMockStateRouter's mergeS3IntoAWSState
	// uses the same authoritative source (the s3 backend); the merge
	// path is verified separately by unit tests in internal/cli.
	if !s3BucketExists(t, s3.URL, "fs-assets-bucket") {
		t.Errorf("expected fs-assets-bucket to exist in s3 backend after apply")
	}

	// Final destroy run cleans up and exercises the destruction
	// acceptance criterion (no_orphans).
	final := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath},
		GeneratorFiles: files,
	})
	if final.Err != nil {
		t.Fatalf("final run failed: %v\nstdout:\n%s", final.Err, final.Stdout)
	}
	if !strings.Contains(final.Stdout, "run/terminal_reason: pass (target_reached)") {
		t.Fatalf("expected final run to reach target_reached, got:\n%s", final.Stdout)
	}
}

// awsStateItemCount returns len(state[root][collection]). fakeaws's
// /mock/state shape: per-service map of slice-valued collections.
// Items inside use AWS-shaped keys (id, arn, name) but we just count.
func awsStateItemCount(state map[string]any, root, collection string) int {
	rootMap, ok := state[root].(map[string]any)
	if !ok {
		return 0
	}
	items, ok := rootMap[collection].([]any)
	if !ok {
		return 0
	}
	return len(items)
}

// s3BucketExists probes the third-party S3 backend with HEAD
// /<bucket> and returns true on 200. M59 assertion path: SeaweedFS's
// anonymous-mode ListAllMyBuckets returns empty even when buckets
// exist (no owner), so listing is not a reliable existence check;
// HEAD-by-name is. The test pins specific bucket names in storage.tf
// so the assertion knows what to ask for.
func s3BucketExists(t *testing.T, s3URL, bucket string) bool {
	t.Helper()
	req, err := http.NewRequest(http.MethodHead, s3URL+"/"+bucket, nil)
	if err != nil {
		t.Fatalf("s3 head bucket: build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("s3 head bucket: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// awsProviderTF returns the provider block. iam/ec2/eks point at
// fakeaws; s3 points at the third-party S3 backend (SeaweedFS,
// M59). s3_use_path_style is required since SeaweedFS uses
// path-style URLs.
func awsProviderTF(fakeawsURL, s3URL string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "fake"
  secret_key                  = "fake"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true
  endpoints {
    iam            = "%[1]s/iam"
    ec2            = "%[1]s/ec2/region/us-east-1"
    eks            = "%[1]s/eks/region/us-east-1"
    rds            = "%[1]s/rds/region/us-east-1"
    secretsmanager = "%[1]s/secretsmanager/region/us-east-1"
    s3             = "%[2]s"
  }
}
`, fakeawsURL, s3URL)
}

func awsFullStackFiles(fakeawsURL, s3URL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(awsProviderTF(fakeawsURL, s3URL)),
		"network.tf": []byte(`resource "aws_vpc" "main" {
  cidr_block = "10.60.0.0/16"
}

resource "aws_subnet" "a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.60.1.0/24"
  availability_zone = "us-east-1a"
}

resource "aws_subnet" "b" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.60.2.0/24"
  availability_zone = "us-east-1b"
}
`),
		"iam.tf": []byte(`resource "aws_iam_role" "cluster" {
  name = "fs-eks-cluster"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "eks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role" "node" {
  name = "fs-eks-node"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}
`),
		"eks.tf": []byte(`resource "aws_eks_cluster" "main" {
  name     = "fs-cluster"
  role_arn = aws_iam_role.cluster.arn
  version  = "1.29"
  vpc_config {
    subnet_ids = [aws_subnet.a.id, aws_subnet.b.id]
  }
}

resource "aws_eks_node_group" "default" {
  cluster_name    = aws_eks_cluster.main.name
  node_group_name = "default"
  node_role_arn   = aws_iam_role.node.arn
  subnet_ids      = [aws_subnet.a.id, aws_subnet.b.id]
  scaling_config {
    desired_size = 1
    min_size     = 1
    max_size     = 2
  }
  depends_on = [aws_eks_cluster.main]
}
`),
		// S3 — exercised against the third-party S3 backend
		// (SeaweedFS, M59). SSE configuration is intentionally
		// minimal (just AES256) since the OPA encryption-at-rest
		// policy is satisfied by the presence of the sibling SSE-
		// config resource alone.
		"storage.tf": []byte(`resource "aws_s3_bucket" "assets" {
  bucket        = "fs-assets-bucket"
  force_destroy = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "assets" {
  # Literal bucket name (not aws_s3_bucket.assets.id) so the OPA
  # encryption-at-rest policy can resolve cfg.values.bucket at plan
  # time. .id is computed and shows as null in planned_values.
  bucket = "fs-assets-bucket"

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }

  depends_on = [aws_s3_bucket.assets]
}
`),
		// RDS — DB subnet group + parameter group + Postgres
		// instance. M61 closed the apply-wait hang + idempotent
		// plan + clean destroy by (a) returning the user-supplied
		// MasterUsername / Port / StorageEncrypted so the
		// provider doesn't see drift and force a replace,
		// (b) emitting DBInstanceNotFound / DBSubnetGroupNotFoundFault
		// instead of the generic ResourceNotFoundException so the
		// destroy-wait state machine recognises completion,
		// (c) returning the deleted DB snapshot inside a
		// <DeleteDBInstanceResult><DBInstance> envelope the SDK can
		// deserialise, and (d) parsing the
		// Filters.Filter.N.Name=dbi-resource-id lookup the provider
		// uses after Create.
		"rds.tf": []byte(`resource "aws_db_subnet_group" "main" {
  name       = "fs-db-sng"
  subnet_ids = [aws_subnet.a.id, aws_subnet.b.id]
}

resource "aws_db_parameter_group" "main" {
  name   = "fs-pg15"
  family = "postgres15"
}

resource "aws_db_instance" "main" {
  identifier           = "fs-db"
  engine               = "postgres"
  engine_version       = "15.4"
  instance_class       = "db.t3.micro"
  allocated_storage    = 20
  username             = "appuser"
  password             = "changeme-not-secret"
  db_subnet_group_name = aws_db_subnet_group.main.name
  parameter_group_name = aws_db_parameter_group.main.name
  skip_final_snapshot  = true
  deletion_protection  = false
  storage_encrypted    = true
}
`),
		// Secrets Manager — secret + initial version. M62 closed
		// the lifecycle by (a) accepting ARN-or-name as SecretId in
		// the repository layer (terraform-provider-aws returns the
		// ARN from CreateSecret and uses it as the id on every
		// subsequent call), (b) returning epoch timestamps for
		// CreatedDate / LastChangedDate / DeletionDate (the SDK
		// rejects strings), (c) populating VersionIdsToStages so
		// the version provider can find AWSCURRENT, and (d)
		// implementing GetResourcePolicy / ListSecretVersionIds /
		// no-op TagResource so refresh doesn't 404.
		// recovery_window_in_days=0 makes destroy synchronous; the
		// default 30-day window leaves the secret in PendingDeletion
		// and breaks no_orphans.
		"secrets.tf": []byte(`resource "aws_secretsmanager_secret" "db" {
  name                    = "fs-db-password"
  description             = "fakeaws full-stack DB password"
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "db" {
  secret_id     = aws_secretsmanager_secret.db.id
  secret_string = "changeme-not-secret"
}
`),
	}
}
