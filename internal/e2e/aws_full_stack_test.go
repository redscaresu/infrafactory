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
// Currently SKIPPED — fakeaws ships a Query-RPC response envelope
// that doesn't match the AWS provider's per-service XML parser.
// Investigation summary (2026-05-24):
//
//  1. fakeaws's WriteQueryRPCResponse uses the IAM-style envelope —
//     <{Action}Response><{Action}Result>...payload...</...></...>—
//     for EVERY service. EC2's real wire shape has no Result wrapper:
//     <{Action}Response><requestId/><vpc>...</vpc></{Action}Response>.
//     terraform-provider-aws's EC2 parser can't find <vpc> nested
//     two levels deep, so output.Vpc comes back nil and the provider
//     panics in resourceVPCCreate at vpc_.go:235.
//  2. fakeaws's typed XML helper also leaks the Go type name as a
//     wrapper element (e.g. <ec2CreateVpcResult> around <vpc>) — a
//     second mismatch on top of the envelope issue. The IAM-side
//     equivalent (ListRolePolicies etc.) was worked around in
//     fakeaws@fea333e by writing the XML inline; same approach would
//     be needed for every EC2 / RDS handler.
//  3. ec2VpcXML was extended in this PR with DhcpOptionsId,
//     InstanceTenancy, OwnerId, CidrBlockAssociationSet,
//     Ipv6CidrBlockAssociationSet — the standard EC2 VPC fields the
//     provider Read flow reads. These improvements stand even though
//     they don't unblock the test alone.
//
// Closing this requires a per-service envelope change in fakeaws's
// awsproto.WriteQueryRPCResponse (EC2 uses no Result wrapper; IAM
// and RDS do), plus inline-XML rewrites of every handler that
// currently leaks its Go type name. Real work, separate from this
// PR.
//
// Each AWS resource in this scenario is exercised individually by
// TestE2E_AWS_{IAM,S3,VPC,Instance,SecurityGroup,RDS,DynamoDB,EKS,
// SQS,Route53,SecretsManager} via direct HTTP — those all pass
// because direct-HTTP tests only assert on body fragments, never
// parse the envelope.
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1 and requires `tofu` on PATH.
func TestE2E_AWSFullStack(t *testing.T) {
	SkipUnlessEnabled(t)
	t.Skip("fakeaws envelope rewrite (M51) landed — provider no longer crashes on parsing. New blocker: per-resource Read-flow field parity. ec2DescribeSubnets returns a stripped-down subnet that's missing fields the provider's Read polls for (availableIpAddressCount, ownerId, subnetArn, mapPublicIpOnLaunch, ipv6CidrBlockAssociationSet, etc.) → 'couldn't find resource (21 retries)'. Same gap exists for EKS / RDS / S3 / Secrets Manager Read flows. Each resource needs ~5-15 standard fields plumbed through ec2SubnetXML / eksClusterJSON / rdsDBInstanceXML / etc. Substantial incremental work; tracked in BACKLOG via the next M-ticket. Un-skip after per-resource field parity catches up.")
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
