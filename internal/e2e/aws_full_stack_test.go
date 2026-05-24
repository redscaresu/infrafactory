package e2e

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_AWSFullStack runs the aws-full-stack training scenario
// composition against fakeaws: VPC + 2 subnets, IAM cluster + node
// roles, EKS cluster + node group, private RDS Postgres, S3 bucket,
// and a Secrets Manager secret. Mirrors TestE2E_FullStackParis
// (Scaleway) and TestE2E_GCPFullStack (GCP) — same lifecycle
// contract.
//
// Currently SKIPPED — known fakeaws compatibility gaps that crash
// the AWS provider (terraform-provider-aws v5.100) when this
// composition is applied through tofu:
//
//  1. ec2.resourceVPCCreate panics with nil pointer dereference at
//     vpc_.go:235 when fakeaws's CreateVpc response is missing one
//     of the fields the provider expects to be non-nil
//     (CidrBlockAssociationSet, Ipv6CidrBlockAssociationSet, or
//     similar). Bug surfaces only on a tofu drive — the direct-HTTP
//     TestE2E_AWS_VPC test bypasses the provider Read flow.
//  2. Likely additional EKS post-create Read-flow gaps; iam handler
//     parity was advanced in this PR (ListRolePolicies, ListRoleTags,
//     ListInstanceProfilesForRole added with inline-XML to avoid the
//     xml.Encoder type-wrapper that crashes the provider plugin).
//
// Each AWS resource in this scenario is exercised individually by
// TestE2E_AWS_{IAM,S3,VPC,Instance,SecurityGroup,RDS,DynamoDB,EKS,
// SQS,Route53,SecretsManager} via direct HTTP — those all pass. The
// composition gap is tracked separately; un-skip this test as each
// gap closes.
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1 and requires `tofu` on PATH.
func TestE2E_AWSFullStack(t *testing.T) {
	SkipUnlessEnabled(t)
	t.Skip("fakeaws: ec2.resourceVPCCreate crashes provider plugin (nil pointer at vpc_.go:235) when full-stack HCL is applied via tofu. Direct-HTTP TestE2E_AWS_VPC passes; gap is in the provider Read flow shape. Un-skip after fakeaws CreateVpc response is brought to parity.")
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakeaws(t)

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := repoScenariosPath(t, "aws-full-stack.yaml")

	WriteConfigMultiCloud(t, configPath, "http://127.0.0.1:1", "", mock.URL, outputRoot)

	files := awsFullStackFiles(mock.URL)

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
		{root: "rds", collection: "db_subnet_groups", minCount: 1},
		{root: "rds", collection: "db_instances", minCount: 1},
		{root: "s3", collection: "buckets", minCount: 1},
		{root: "secretsmanager", collection: "secrets", minCount: 1},
	} {
		got := awsStateItemCount(state, exp.root, exp.collection)
		if got < exp.minCount {
			t.Errorf("expected at least %d %s/%s after apply, got %d",
				exp.minCount, exp.root, exp.collection, got)
		}
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

// awsProviderTF returns the provider block pointing every used
// service at the per-test fakeaws URL. Mirrors the per-example
// pattern in ../fakeaws/examples/working/*/main.tf — single source
// of truth is the fakeaws repo's example providers.
func awsProviderTF(fakeawsURL string) string {
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
  endpoints {
    iam            = "%[1]s/iam"
    ec2            = "%[1]s/ec2/region/us-east-1"
    eks            = "%[1]s/eks/region/us-east-1"
    rds            = "%[1]s/rds/region/us-east-1"
    s3             = "%[1]s/s3"
    secretsmanager = "%[1]s/secretsmanager/region/us-east-1"
  }
}
`, fakeawsURL)
}

func awsFullStackFiles(fakeawsURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(awsProviderTF(fakeawsURL)),
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
		"rds.tf": []byte(`resource "aws_db_subnet_group" "default" {
  name       = "fs-db-subnets"
  subnet_ids = [aws_subnet.a.id, aws_subnet.b.id]
}

resource "aws_db_parameter_group" "pg15" {
  name   = "fs-pg15"
  family = "postgres15"
}

resource "aws_db_instance" "app" {
  identifier           = "fs-app-db"
  engine               = "postgres"
  engine_version       = "15.4"
  instance_class       = "db.t3.micro"
  allocated_storage    = 20
  username             = "appuser"
  password             = "changeme"
  db_subnet_group_name = aws_db_subnet_group.default.name
  parameter_group_name = aws_db_parameter_group.pg15.name
  skip_final_snapshot  = true
  deletion_protection  = false
  storage_encrypted    = true
}
`),
		"storage.tf": []byte(`resource "aws_s3_bucket" "assets" {
  bucket        = "fs-assets-bucket"
  force_destroy = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "assets" {
  # Use the literal bucket name (not aws_s3_bucket.assets.id) so the
  # OPA encryption policy can resolve cfg.values.bucket at plan
  # time — bucket name is known statically, but .id is computed
  # and shows as null in planned_values.
  bucket = "fs-assets-bucket"

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }

  depends_on = [aws_s3_bucket.assets]
}
`),
		"secrets.tf": []byte(`resource "aws_secretsmanager_secret" "db" {
  name                    = "fs-db-creds"
  description             = "Full-stack app database credentials"
  recovery_window_in_days = 0
}
`),
	}
}
