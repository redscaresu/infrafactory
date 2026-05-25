package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// ministackStateBuilder is the M65 polyfill: it walks ministack's
// AWS-SDK introspection APIs in parallel and synthesizes a JSON blob
// in the same shape that fakeaws's GET /mock/state returns. The shape
// contract is derived from fakeaws/handlers/admin.go — see the comment
// on emptyMinistackState for the per-service collection list.
//
// Why a builder rather than reaching for the upstream SDK ad-hoc: this
// keeps the mock-state interface (Reset/Snapshot/Restore/State) clean
// across both the legacy mockStateClient (mockway/fakegcp/fakeaws) and
// the new ministackClient. ministackClient.State() calls
// BuildState(ctx, baseURL) and otherwise has no knowledge of which
// AWS services we care about.
//
// Consumers of the JSON:
//   - countOrphans in internal/harness/destroy.go — walks
//     state[root][collection] arrays counting length (only the array
//     lengths matter for destruction no_orphans; element content
//     can be minimal).
//   - OPA deny_state rules in policies/aws/ — currently only
//     no_public_db.rego reads input.rds.instances[].publicly_accessible
//     so RDS rows must include that field.
//   - Test-side awsStateItemCount helper in internal/e2e/.
//
// Concurrency: each service call is independent and runs in its own
// goroutine. A failure in any one service produces an empty array for
// that service block; the overall State() call does not fail because
// of a single service hiccup (otherwise a transient ListSecrets timeout
// would break every Layer 2 check).
type defaultMinistackStateBuilder struct{}

func newMinistackStateBuilder() ministackStateBuilder {
	return &defaultMinistackStateBuilder{}
}

func (b *defaultMinistackStateBuilder) BuildState(ctx context.Context, baseURL string) ([]byte, error) {
	awsCfg := ministackAWSConfig(baseURL)

	// Per-service result buckets. Each goroutine writes into its slot
	// under a single shared mutex (the writes are after the network
	// call, so contention is minimal).
	state := emptyMinistackState()
	var mu sync.Mutex
	var wg sync.WaitGroup

	probes := []struct {
		name string
		run  func(context.Context, aws.Config) (string, map[string]any)
	}{
		{"ec2", probeMinistackEC2},
		{"iam", probeMinistackIAM},
		{"rds", probeMinistackRDS},
		{"eks", probeMinistackEKS},
		{"s3", probeMinistackS3},
		{"secretsmanager", probeMinistackSecretsManager},
	}

	for _, p := range probes {
		wg.Add(1)
		go func(p struct {
			name string
			run  func(context.Context, aws.Config) (string, map[string]any)
		}) {
			defer wg.Done()
			root, payload := p.run(ctx, awsCfg)
			if payload == nil {
				return
			}
			mu.Lock()
			state[root] = payload
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	return json.Marshal(state)
}

// emptyMinistackState mirrors fakeaws's GET /mock/state empty shape so
// countOrphans + OPA + tests see the expected top-level keys even when
// a service probe failed and contributed nothing. Schema version pin
// matches fakeaws/handlers/admin.go.
func emptyMinistackState() map[string]any {
	return map[string]any{
		"audit": []any{},
		"dynamodb": map[string]any{
			"items":  []any{},
			"tables": []any{},
		},
		"ec2": map[string]any{
			"eips":                     []any{},
			"instances":                []any{},
			"internet_gateways":        []any{},
			"key_pairs":                []any{},
			"route_table_associations": []any{},
			"route_tables":             []any{},
			"routes":                   []any{},
			"security_groups":          []any{},
			"subnets":                  []any{},
			"vpcs":                     []any{},
		},
		"eks": map[string]any{
			"addons":      []any{},
			"clusters":    []any{},
			"node_groups": []any{},
		},
		"iam": map[string]any{
			"access_keys":       []any{},
			"instance_profiles": []any{},
			"policies":          []any{},
			"roles":             []any{},
			"users":             []any{},
		},
		"operations": []any{},
		"rds": map[string]any{
			"db_cluster_parameter_groups": []any{},
			"db_clusters":                 []any{},
			"db_instances":                []any{},
			"db_parameter_groups":         []any{},
			"db_subnet_groups":            []any{},
		},
		"route53": map[string]any{
			"hosted_zones": []any{},
			"record_sets":  []any{},
		},
		"s3": map[string]any{
			"buckets": []any{},
		},
		"schema_version": 1,
		"secretsmanager": map[string]any{
			"secrets":  []any{},
			"versions": []any{},
		},
		"sqs": map[string]any{
			"messages":            []any{},
			"queues":              []any{},
			"tombstoned_messages": 0,
		},
	}
}

// ministackAWSConfig returns an aws.Config wired to talk to a local
// ministack instance: credentials are placeholder strings, region is
// us-east-1 (every ministack instance uses this as the default
// region), and the BaseEndpoint override sends every service to the
// same host:port that ministack listens on.
func ministackAWSConfig(baseURL string) aws.Config {
	baseURL = strings.TrimRight(baseURL, "/")
	return aws.Config{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
		BaseEndpoint: aws.String(baseURL),
	}
}

// --- EC2 ---

func probeMinistackEC2(ctx context.Context, cfg aws.Config) (string, map[string]any) {
	client := ec2.NewFromConfig(cfg)
	out := map[string]any{
		"eips":                     []any{},
		"instances":                []any{},
		"internet_gateways":        []any{},
		"key_pairs":                []any{},
		"route_table_associations": []any{},
		"route_tables":             []any{},
		"routes":                   []any{},
		"security_groups":          []any{},
		"subnets":                  []any{},
		"vpcs":                     []any{},
	}

	if resp, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.Vpcs))
		for _, v := range resp.Vpcs {
			rows = append(rows, map[string]any{
				"id":         aws.ToString(v.VpcId),
				"cidr_block": aws.ToString(v.CidrBlock),
			})
		}
		out["vpcs"] = rows
	}
	if resp, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.Subnets))
		for _, s := range resp.Subnets {
			rows = append(rows, map[string]any{
				"id":                aws.ToString(s.SubnetId),
				"vpc_id":            aws.ToString(s.VpcId),
				"cidr_block":        aws.ToString(s.CidrBlock),
				"availability_zone": aws.ToString(s.AvailabilityZone),
			})
		}
		out["subnets"] = rows
	}
	if resp, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.SecurityGroups))
		for _, sg := range resp.SecurityGroups {
			rows = append(rows, map[string]any{
				"id":     aws.ToString(sg.GroupId),
				"name":   aws.ToString(sg.GroupName),
				"vpc_id": aws.ToString(sg.VpcId),
			})
		}
		out["security_groups"] = rows
	}
	if resp, err := client.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.InternetGateways))
		for _, ig := range resp.InternetGateways {
			rows = append(rows, map[string]any{"id": aws.ToString(ig.InternetGatewayId)})
		}
		out["internet_gateways"] = rows
	}
	return "ec2", out
}

// --- IAM ---

func probeMinistackIAM(ctx context.Context, cfg aws.Config) (string, map[string]any) {
	client := iam.NewFromConfig(cfg)
	out := map[string]any{
		"access_keys":       []any{},
		"instance_profiles": []any{},
		"policies":          []any{},
		"roles":             []any{},
		"users":             []any{},
	}

	if resp, err := client.ListRoles(ctx, &iam.ListRolesInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.Roles))
		for _, r := range resp.Roles {
			rows = append(rows, map[string]any{
				"name": aws.ToString(r.RoleName),
				"arn":  aws.ToString(r.Arn),
			})
		}
		out["roles"] = rows
	}
	if resp, err := client.ListPolicies(ctx, &iam.ListPoliciesInput{Scope: "Local"}); err == nil {
		rows := make([]map[string]any, 0, len(resp.Policies))
		for _, p := range resp.Policies {
			rows = append(rows, map[string]any{
				"name": aws.ToString(p.PolicyName),
				"arn":  aws.ToString(p.Arn),
			})
		}
		out["policies"] = rows
	}
	if resp, err := client.ListUsers(ctx, &iam.ListUsersInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.Users))
		for _, u := range resp.Users {
			rows = append(rows, map[string]any{
				"name": aws.ToString(u.UserName),
				"arn":  aws.ToString(u.Arn),
			})
		}
		out["users"] = rows
	}
	return "iam", out
}

// --- RDS ---

func probeMinistackRDS(ctx context.Context, cfg aws.Config) (string, map[string]any) {
	client := rds.NewFromConfig(cfg)
	out := map[string]any{
		"db_cluster_parameter_groups": []any{},
		"db_clusters":                 []any{},
		"db_instances":                []any{},
		"db_parameter_groups":         []any{},
		"db_subnet_groups":            []any{},
	}

	if resp, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.DBInstances))
		for _, inst := range resp.DBInstances {
			rows = append(rows, map[string]any{
				"name":                 aws.ToString(inst.DBInstanceIdentifier),
				"engine":               aws.ToString(inst.Engine),
				"publicly_accessible":  aws.ToBool(inst.PubliclyAccessible),
				"storage_encrypted":    aws.ToBool(inst.StorageEncrypted),
				"db_subnet_group_name": rdsSubnetGroupName(inst.DBSubnetGroup),
			})
		}
		out["db_instances"] = rows
	}
	if resp, err := client.DescribeDBSubnetGroups(ctx, &rds.DescribeDBSubnetGroupsInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.DBSubnetGroups))
		for _, sg := range resp.DBSubnetGroups {
			rows = append(rows, map[string]any{
				"name":   aws.ToString(sg.DBSubnetGroupName),
				"vpc_id": aws.ToString(sg.VpcId),
			})
		}
		out["db_subnet_groups"] = rows
	}
	if resp, err := client.DescribeDBParameterGroups(ctx, &rds.DescribeDBParameterGroupsInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.DBParameterGroups))
		for _, pg := range resp.DBParameterGroups {
			rows = append(rows, map[string]any{
				"name":   aws.ToString(pg.DBParameterGroupName),
				"family": aws.ToString(pg.DBParameterGroupFamily),
			})
		}
		out["db_parameter_groups"] = rows
	}
	if resp, err := client.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{}); err == nil {
		rows := make([]map[string]any, 0, len(resp.DBClusters))
		for _, c := range resp.DBClusters {
			rows = append(rows, map[string]any{
				"name":   aws.ToString(c.DBClusterIdentifier),
				"engine": aws.ToString(c.Engine),
			})
		}
		out["db_clusters"] = rows
	}
	return "rds", out
}

// rdsSubnetGroupName extracts the name field from the SDK's nested
// DBSubnetGroup struct without panicking when it's nil.
func rdsSubnetGroupName(sg any) string {
	if sg == nil {
		return ""
	}
	// rds.types.DBSubnetGroup is a struct with DBSubnetGroupName *string;
	// fall back to JSON round-trip rather than pulling in the types
	// package for a single field access.
	body, err := json.Marshal(sg)
	if err != nil {
		return ""
	}
	var probe struct {
		DBSubnetGroupName *string `json:"DBSubnetGroupName"`
	}
	if json.Unmarshal(body, &probe) != nil || probe.DBSubnetGroupName == nil {
		return ""
	}
	return *probe.DBSubnetGroupName
}

// --- EKS ---

func probeMinistackEKS(ctx context.Context, cfg aws.Config) (string, map[string]any) {
	client := eks.NewFromConfig(cfg)
	out := map[string]any{
		"addons":      []any{},
		"clusters":    []any{},
		"node_groups": []any{},
	}

	listClusters, err := client.ListClusters(ctx, &eks.ListClustersInput{})
	if err != nil {
		return "eks", out
	}
	clusterRows := make([]map[string]any, 0, len(listClusters.Clusters))
	nodeGroupRows := []map[string]any{}
	for _, name := range listClusters.Clusters {
		desc, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: aws.String(name)})
		if err != nil || desc.Cluster == nil {
			clusterRows = append(clusterRows, map[string]any{"name": name})
			continue
		}
		clusterRows = append(clusterRows, map[string]any{
			"name":    aws.ToString(desc.Cluster.Name),
			"arn":     aws.ToString(desc.Cluster.Arn),
			"version": aws.ToString(desc.Cluster.Version),
		})

		ngList, err := client.ListNodegroups(ctx, &eks.ListNodegroupsInput{ClusterName: aws.String(name)})
		if err != nil {
			continue
		}
		for _, ng := range ngList.Nodegroups {
			nodeGroupRows = append(nodeGroupRows, map[string]any{
				"cluster_name":    name,
				"node_group_name": ng,
			})
		}
	}
	out["clusters"] = clusterRows
	out["node_groups"] = nodeGroupRows
	return "eks", out
}

// --- S3 ---

func probeMinistackS3(ctx context.Context, cfg aws.Config) (string, map[string]any) {
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	out := map[string]any{
		"buckets": []any{},
	}
	resp, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return "s3", out
	}
	rows := make([]map[string]any, 0, len(resp.Buckets))
	for _, b := range resp.Buckets {
		rows = append(rows, map[string]any{
			"name": aws.ToString(b.Name),
		})
	}
	out["buckets"] = rows
	return "s3", out
}

// --- Secrets Manager ---

func probeMinistackSecretsManager(ctx context.Context, cfg aws.Config) (string, map[string]any) {
	client := secretsmanager.NewFromConfig(cfg)
	out := map[string]any{
		"secrets":  []any{},
		"versions": []any{},
	}
	resp, err := client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})
	if err != nil {
		return "secretsmanager", out
	}
	secretRows := make([]map[string]any, 0, len(resp.SecretList))
	versionRows := []map[string]any{}
	for _, s := range resp.SecretList {
		secretRows = append(secretRows, map[string]any{
			"name": aws.ToString(s.Name),
			"arn":  aws.ToString(s.ARN),
		})
		// One pseudo-row per secret captures "this secret has a value".
		// Per-version walking would require ListSecretVersionIds — skip
		// for now; consumers only count.
		versionRows = append(versionRows, map[string]any{
			"secret_name": aws.ToString(s.Name),
		})
	}
	out["secrets"] = secretRows
	out["versions"] = versionRows
	return "secretsmanager", out
}

// avoid unused-import errors when adding services incrementally.
var _ = fmt.Sprintf
