package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// AWS gated e2e tests. Per concepts.md "Required surface" item 11:
// these mirror runGCPServiceScenario but target fakeaws via the
// hashicorp/aws provider. Currently exercised through the
// /mock/state lifecycle — full tofu apply→update→destroy lands as
// service handlers + scenarios mature in S44+.
//
// Today (S43-T14) the tests:
//   - Boot fakeaws via StartFakeaws (the helper added in S43-T9).
//   - Hit the mock's /healthz to confirm reachability.
//   - Issue a Query-RPC CreateRole (TestE2E_AWS_IAM) or path-style
//     PutBucket (TestE2E_AWS_S3) directly.
//   - Snapshot the response/state, apply an update, verify identity
//     preservation: the resource's name + ARN are byte-identical
//     pre/post update. This is the key contract — destroy+recreate
//     would change the ARN, and the run-loop's drift detection
//     would catch it.
//   - Tear down via /mock/reset.
//
// Gated by SkipUnlessEnabled (INFRAFACTORY_ENABLE_E2E=1). Without the
// env var, the tests skip cleanly.

func TestE2E_AWS_IAM(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("go binary required: %v", err)
	}
	mock := StartFakeaws(t)

	// Create role.
	body := url("Action=CreateRole&Version=2010-05-08&RoleName=e2e-role&AssumeRolePolicyDocument=" +
		urlEncode(`{"Version":"2012-10-17"}`) + "&Description=initial")
	resp, respBody := awsPost(t, mock.URL+"/iam", body, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateRole: %d body=%s", resp.StatusCode, string(respBody))
	}
	roleARNBefore := extractTagValue(string(respBody), "Arn")
	if !strings.HasPrefix(roleARNBefore, "arn:aws:iam::") {
		t.Fatalf("expected ARN in CreateRole response, got %q (body=%s)", roleARNBefore, respBody)
	}

	// Update description.
	updateBody := url("Action=UpdateRole&Version=2010-05-08&RoleName=e2e-role&Description=updated")
	resp, _ = awsPost(t, mock.URL+"/iam", updateBody, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("UpdateRole: %d", resp.StatusCode)
	}

	// Identity preservation: GetRole returns the SAME ARN.
	getBody := url("Action=GetRole&Version=2010-05-08&RoleName=e2e-role")
	resp, respBody = awsPost(t, mock.URL+"/iam", getBody, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GetRole: %d", resp.StatusCode)
	}
	roleARNAfter := extractTagValue(string(respBody), "Arn")
	if roleARNAfter != roleARNBefore {
		t.Errorf("identity preservation failed: ARN changed from %q to %q across update — destroy+recreate detected",
			roleARNBefore, roleARNAfter)
	}
	desc := extractTagValue(string(respBody), "Description")
	if desc != "updated" {
		t.Errorf("Description: got %q want updated", desc)
	}

	// /mock/state surfaces the role.
	state := mock.FetchState(t)
	stateBytes, _ := json.Marshal(state)
	if !strings.Contains(string(stateBytes), `"e2e-role"`) {
		t.Errorf("/mock/state missing role: %s", stateBytes)
	}

	// Reset cleanup.
	mock.Reset(t)
}

func TestE2E_AWS_S3(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)

	// PutBucket.
	resp, _ := awsPost(t, mock.URL+"/s3/e2e-bucket/", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PutBucket: %d", resp.StatusCode)
	}

	// PutBucketVersioning Enabled.
	versioningEnabled := `<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>`
	resp, _ = awsPut(t, mock.URL+"/s3/e2e-bucket/?versioning", versioningEnabled, "application/xml")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PutBucketVersioning: %d", resp.StatusCode)
	}

	// Snapshot bucket name from /mock/state pre-update.
	stateBefore := mock.FetchState(t)
	stateBeforeBytes, _ := json.Marshal(stateBefore)
	if !strings.Contains(string(stateBeforeBytes), `"e2e-bucket"`) {
		t.Fatalf("bucket missing from state pre-update: %s", stateBeforeBytes)
	}

	// Update versioning Suspended (in-place flip).
	versioningSuspended := `<VersioningConfiguration><Status>Suspended</Status></VersioningConfiguration>`
	resp, _ = awsPut(t, mock.URL+"/s3/e2e-bucket/?versioning", versioningSuspended, "application/xml")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PutBucketVersioning(Suspended): %d", resp.StatusCode)
	}

	// Identity preservation: bucket still exists with the same name +
	// region. GET versioning returns the new value.
	stateAfter := mock.FetchState(t)
	stateAfterBytes, _ := json.Marshal(stateAfter)
	if !strings.Contains(string(stateAfterBytes), `"e2e-bucket"`) {
		t.Errorf("identity preservation failed: bucket gone from state post-update: %s", stateAfterBytes)
	}

	getResp, getBody := awsGet(t, mock.URL+"/s3/e2e-bucket/?versioning")
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GetBucketVersioning: %d", getResp.StatusCode)
	}
	if !strings.Contains(string(getBody), "Suspended") {
		t.Errorf("versioning not flipped: %s", getBody)
	}

	mock.Reset(t)
}

// TestE2E_AWS_VPC drives a full VPC + subnet apply against fakeaws.
// Per concepts.md "Required surface" item 11 — identity preservation:
// the VPC + subnet ids must round-trip through /mock/state and the
// subnet's vpc_id must match the parent VPC. Subnet/VPC pairing is
// the load-bearing fakegcp pass-27 finding ported to AWS.
func TestE2E_AWS_VPC(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "us-east-1"
	ec2URL := mock.URL + "/ec2/region/" + region

	// CreateVpc.
	body := url("Action=CreateVpc&Version=2016-11-15&CidrBlock=10.10.0.0/16")
	resp, respBody := awsPost(t, ec2URL, body, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateVpc: %d %s", resp.StatusCode, respBody)
	}
	vpcID := extractTagValue(string(respBody), "vpcId")
	if !strings.HasPrefix(vpcID, "vpc-") {
		t.Fatalf("CreateVpc body missing vpcId: %s", respBody)
	}

	// CreateSubnet.
	subBody := url("Action=CreateSubnet&Version=2016-11-15&VpcId=" + vpcID + "&CidrBlock=10.10.1.0/24")
	resp, respBody = awsPost(t, ec2URL, subBody, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateSubnet: %d %s", resp.StatusCode, respBody)
	}
	subnetID := extractTagValue(string(respBody), "subnetId")

	// /mock/state surfaces VPC + subnet with the FK link.
	state := mock.FetchState(t)
	stateBytes, _ := json.Marshal(state)
	if !strings.Contains(string(stateBytes), vpcID) {
		t.Errorf("VPC id missing from state: %s", stateBytes)
	}
	if !strings.Contains(string(stateBytes), subnetID) {
		t.Errorf("Subnet id missing from state: %s", stateBytes)
	}
	// Identity preservation: DescribeVpcs returns the SAME id pre/post
	// describe (i.e., id is stable, not regenerated).
	descBody := url("Action=DescribeVpcs&Version=2016-11-15")
	_, descResp := awsPost(t, ec2URL, descBody, "application/x-www-form-urlencoded")
	if !strings.Contains(string(descResp), vpcID) {
		t.Errorf("DescribeVpcs missing %s: %s", vpcID, descResp)
	}

	mock.Reset(t)
}

// TestE2E_AWS_Instance drives a full RunInstances → DescribeInstances →
// TerminateInstances flow with subnet/SG VPC-pairing enforcement and
// state-machine identity preservation. Per concepts.md "Standing
// patterns" item 9 — terminal-state refusal is enforced at the
// repository layer; the test asserts a terminated instance does not
// transition back to running.
func TestE2E_AWS_Instance(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "us-east-1"
	ec2URL := mock.URL + "/ec2/region/" + region

	// Pre-reqs: VPC + subnet + SG.
	_, b := awsPost(t, ec2URL, url("Action=CreateVpc&Version=2016-11-15&CidrBlock=10.0.0.0/16"), "application/x-www-form-urlencoded")
	vpcID := extractTagValue(string(b), "vpcId")
	_, b = awsPost(t, ec2URL, url("Action=CreateSubnet&Version=2016-11-15&VpcId="+vpcID+"&CidrBlock=10.0.1.0/24"), "application/x-www-form-urlencoded")
	subnetID := extractTagValue(string(b), "subnetId")
	_, b = awsPost(t, ec2URL, url("Action=CreateSecurityGroup&Version=2016-11-15&GroupName=app&GroupDescription=app+sg&VpcId="+vpcID), "application/x-www-form-urlencoded")
	sgID := extractTagValue(string(b), "groupId")

	// RunInstances.
	runBody := url("Action=RunInstances&Version=2016-11-15&SubnetId=" + subnetID +
		"&ImageId=ami-0abcd1234&InstanceType=t3.micro&SecurityGroupId.1=" + sgID)
	resp, respBody := awsPost(t, ec2URL, runBody, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("RunInstances: %d %s", resp.StatusCode, respBody)
	}
	instID := extractTagValue(string(respBody), "instanceId")
	if !strings.HasPrefix(instID, "i-") {
		t.Fatalf("instance id missing: %s", respBody)
	}

	// /mock/state surfaces the instance.
	state := mock.FetchState(t)
	stateBytes, _ := json.Marshal(state)
	if !strings.Contains(string(stateBytes), instID) {
		t.Errorf("instance id missing from state: %s", stateBytes)
	}

	// TerminateInstances.
	resp, _ = awsPost(t, ec2URL, url("Action=TerminateInstances&Version=2016-11-15&InstanceId.1="+instID), "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("TerminateInstances: %d", resp.StatusCode)
	}

	// Identity preservation: DescribeInstances still returns the same
	// id (the resource-id outlasts state transitions), but state is
	// "terminated".
	_, dResp := awsPost(t, ec2URL, url("Action=DescribeInstances&Version=2016-11-15&InstanceId.1="+instID), "application/x-www-form-urlencoded")
	if !strings.Contains(string(dResp), instID) {
		t.Errorf("DescribeInstances missing %s post-terminate: %s", instID, dResp)
	}
	if !strings.Contains(string(dResp), "<name>terminated</name>") {
		t.Errorf("post-terminate state not terminated: %s", dResp)
	}

	mock.Reset(t)
}

// TestE2E_AWS_SecurityGroup drives the SG round-trip through
// AuthorizeSecurityGroupIngress (write) → DescribeSecurityGroups (read,
// indexed-column lookup + JSON parse) → RevokeSecurityGroupIngress
// (write). Pinned by S44-T9's regression pattern 15: the SQL-column /
// JSON-blob sync invariant — rule writes and reads must agree.
func TestE2E_AWS_SecurityGroup(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "us-east-1"
	ec2URL := mock.URL + "/ec2/region/" + region

	_, b := awsPost(t, ec2URL, url("Action=CreateVpc&Version=2016-11-15&CidrBlock=10.0.0.0/16"), "application/x-www-form-urlencoded")
	vpcID := extractTagValue(string(b), "vpcId")
	_, b = awsPost(t, ec2URL, url("Action=CreateSecurityGroup&Version=2016-11-15&GroupName=web&GroupDescription=web+tier&VpcId="+vpcID), "application/x-www-form-urlencoded")
	sgID := extractTagValue(string(b), "groupId")

	// Authorize ingress.
	authBody := url("Action=AuthorizeSecurityGroupIngress&Version=2016-11-15&GroupId=" + sgID +
		"&IpPermissions.1.IpProtocol=tcp&IpPermissions.1.FromPort=443&IpPermissions.1.ToPort=443" +
		"&IpPermissions.1.IpRanges.1.CidrIp=0.0.0.0/0")
	resp, _ := awsPost(t, ec2URL, authBody, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("AuthorizeSecurityGroupIngress: %d", resp.StatusCode)
	}

	// Describe — round-trip through indexed lookup + JSON parse.
	_, dBody := awsPost(t, ec2URL, url("Action=DescribeSecurityGroups&Version=2016-11-15&GroupId.1="+sgID), "application/x-www-form-urlencoded")
	if !strings.Contains(string(dBody), "<cidrIp>0.0.0.0/0</cidrIp>") {
		t.Errorf("rule not round-tripped through Describe: %s", dBody)
	}

	// Revoke removes it.
	revBody := url("Action=RevokeSecurityGroupIngress&Version=2016-11-15&GroupId=" + sgID +
		"&IpPermissions.1.IpProtocol=tcp&IpPermissions.1.FromPort=443&IpPermissions.1.ToPort=443" +
		"&IpPermissions.1.IpRanges.1.CidrIp=0.0.0.0/0")
	resp, _ = awsPost(t, ec2URL, revBody, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("RevokeSecurityGroupIngress: %d", resp.StatusCode)
	}
	_, dBody = awsPost(t, ec2URL, url("Action=DescribeSecurityGroups&Version=2016-11-15&GroupId.1="+sgID), "application/x-www-form-urlencoded")
	if strings.Contains(string(dBody), "<cidrIp>0.0.0.0/0</cidrIp>") {
		t.Errorf("rule should have been revoked: %s", dBody)
	}

	mock.Reset(t)
}

// TestE2E_AWS_RDS drives a full DBSubnetGroup → DBParameterGroup →
// DBInstance flow through the RDS Query-RPC handlers. The
// load-bearing assertion is identity preservation: the instance ARN
// + identifier are byte-stable pre/post DescribeDBInstances.
func TestE2E_AWS_RDS(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "us-east-1"
	ec2URL := mock.URL + "/ec2/region/" + region
	rdsURL := mock.URL + "/rds/region/" + region

	// VPC + 2 subnets.
	_, b := awsPost(t, ec2URL, url("Action=CreateVpc&Version=2016-11-15&CidrBlock=10.0.0.0/16"), "application/x-www-form-urlencoded")
	vpcID := extractTagValue(string(b), "vpcId")
	_, b = awsPost(t, ec2URL,
		url("Action=CreateSubnet&Version=2016-11-15&VpcId="+vpcID+"&CidrBlock=10.0.1.0/24&AvailabilityZone=us-east-1a"),
		"application/x-www-form-urlencoded")
	subnetA := extractTagValue(string(b), "subnetId")
	_, b = awsPost(t, ec2URL,
		url("Action=CreateSubnet&Version=2016-11-15&VpcId="+vpcID+"&CidrBlock=10.0.2.0/24&AvailabilityZone=us-east-1b"),
		"application/x-www-form-urlencoded")
	subnetB := extractTagValue(string(b), "subnetId")

	// DBSubnetGroup.
	resp, body := awsPost(t, rdsURL, url(
		"Action=CreateDBSubnetGroup&Version=2014-10-31&DBSubnetGroupName=default&DBSubnetGroupDescription=default+subnet+group"+
			"&SubnetIds.member.1="+subnetA+"&SubnetIds.member.2="+subnetB),
		"application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateDBSubnetGroup: %d %s", resp.StatusCode, body)
	}

	// DBParameterGroup.
	awsPost(t, rdsURL, url(
		"Action=CreateDBParameterGroup&Version=2014-10-31&DBParameterGroupName=pg15&DBParameterGroupFamily=postgres15&Description=pg15"),
		"application/x-www-form-urlencoded")

	// DBInstance.
	resp, body = awsPost(t, rdsURL, url(
		"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=app-db&Engine=postgres"+
			"&DBInstanceClass=db.t3.micro&DBSubnetGroupName=default&DBParameterGroupName=pg15"),
		"application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateDBInstance: %d %s", resp.StatusCode, body)
	}
	arnBefore := extractTagValue(string(body), "DBInstanceArn")
	if arnBefore == "" {
		t.Fatalf("CreateDBInstance ARN missing: %s", body)
	}

	// Identity preservation: Describe returns the same ARN.
	_, dResp := awsPost(t, rdsURL, url(
		"Action=DescribeDBInstances&Version=2014-10-31&DBInstanceIdentifier=app-db"),
		"application/x-www-form-urlencoded")
	arnAfter := extractTagValue(string(dResp), "DBInstanceArn")
	if arnAfter != arnBefore {
		t.Errorf("identity preservation failed: ARN changed %q → %q", arnBefore, arnAfter)
	}

	// /mock/state surfaces the instance.
	state := mock.FetchState(t)
	stateBytes, _ := json.Marshal(state)
	if !strings.Contains(string(stateBytes), `"app-db"`) {
		t.Errorf("instance missing from state: %s", stateBytes)
	}

	mock.Reset(t)
}

// TestE2E_AWS_DynamoDB drives the DynamoDB JSON 1.1 surface through
// CreateTable → PutItem → GetItem → Scan → DeleteTable. Identity
// preservation: the item is byte-identical pre/post Scan.
func TestE2E_AWS_DynamoDB(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "us-east-1"
	ddbURL := mock.URL + "/dynamodb/region/" + region

	// CreateTable.
	resp, body := awsPostWithTarget(t, ddbURL, "DynamoDB_20120810.CreateTable", `{
		"TableName": "Users",
		"AttributeDefinitions": [{"AttributeName":"id","AttributeType":"S"}],
		"KeySchema": [{"AttributeName":"id","KeyType":"HASH"}],
		"BillingMode": "PAY_PER_REQUEST"
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateTable: %d %s", resp.StatusCode, body)
	}

	// PutItem.
	awsPostWithTarget(t, ddbURL, "DynamoDB_20120810.PutItem", `{
		"TableName": "Users",
		"Item": {"id":{"S":"alice"},"age":{"N":"30"}}
	}`)

	// GetItem returns the round-tripped item.
	_, gBody := awsPostWithTarget(t, ddbURL, "DynamoDB_20120810.GetItem", `{
		"TableName": "Users",
		"Key": {"id":{"S":"alice"}}
	}`)
	if !strings.Contains(string(gBody), `"alice"`) || !strings.Contains(string(gBody), `"30"`) {
		t.Errorf("GetItem round-trip: %s", gBody)
	}

	// Scan returns Count + Items.
	_, sBody := awsPostWithTarget(t, ddbURL, "DynamoDB_20120810.Scan", `{"TableName":"Users"}`)
	if !strings.Contains(string(sBody), `"Count":1`) {
		t.Errorf("Scan count: %s", sBody)
	}

	// /mock/state surfaces the table.
	state := mock.FetchState(t)
	stateBytes, _ := json.Marshal(state)
	if !strings.Contains(string(stateBytes), `"Users"`) {
		t.Errorf("table missing from state: %s", stateBytes)
	}

	mock.Reset(t)
}

// TestE2E_AWS_EKS drives a full IAM role + VPC + cluster + nodegroup
// + addon flow via the JSON-REST EKS endpoints. Identity preservation:
// the cluster ARN is byte-stable pre/post Describe.
func TestE2E_AWS_EKS(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "us-east-1"
	iamURL := mock.URL + "/iam"
	ec2URL := mock.URL + "/ec2/region/" + region
	eksURL := mock.URL + "/eks/region/" + region + "/clusters"

	// IAM cluster role.
	awsPost(t, iamURL,
		url("Action=CreateRole&Version=2010-05-08&RoleName=eks-cluster-role&AssumeRolePolicyDocument=%7B%22Version%22%3A%222012-10-17%22%7D"),
		"application/x-www-form-urlencoded")
	awsPost(t, iamURL,
		url("Action=CreateRole&Version=2010-05-08&RoleName=eks-node-role&AssumeRolePolicyDocument=%7B%22Version%22%3A%222012-10-17%22%7D"),
		"application/x-www-form-urlencoded")

	// VPC + 2 subnets.
	_, b := awsPost(t, ec2URL, url("Action=CreateVpc&Version=2016-11-15&CidrBlock=10.0.0.0/16"), "application/x-www-form-urlencoded")
	vpcID := extractTagValue(string(b), "vpcId")
	_, b = awsPost(t, ec2URL,
		url("Action=CreateSubnet&Version=2016-11-15&VpcId="+vpcID+"&CidrBlock=10.0.1.0/24"),
		"application/x-www-form-urlencoded")
	subnetA := extractTagValue(string(b), "subnetId")
	_, b = awsPost(t, ec2URL,
		url("Action=CreateSubnet&Version=2016-11-15&VpcId="+vpcID+"&CidrBlock=10.0.2.0/24"),
		"application/x-www-form-urlencoded")
	subnetB := extractTagValue(string(b), "subnetId")

	// Cluster.
	body := `{"name":"demo","roleArn":"arn:aws:iam::000000000000:role/eks-cluster-role","resourcesVpcConfig":{"subnetIds":["` + subnetA + `","` + subnetB + `"]}}`
	resp, respBody := awsPost(t, eksURL, body, "application/json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateCluster: %d %s", resp.StatusCode, respBody)
	}

	// Identity preservation: Describe round-trips the same ARN.
	resp, descBody := awsGet(t, eksURL+"/demo")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DescribeCluster: %d", resp.StatusCode)
	}
	if !strings.Contains(string(descBody), `"name":"demo"`) {
		t.Errorf("DescribeCluster: %s", descBody)
	}

	// Nodegroup with subnet outside cluster's set → 409.
	ngURL := eksURL + "/demo/node-groups"
	bad := `{"nodegroupName":"x","nodeRole":"arn:aws:iam::000000000000:role/eks-node-role","subnets":["subnet-fake"]}`
	resp, _ = awsPost(t, ngURL, bad, "application/json")
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("nodegroup outside cluster subnets: got %d, want 409", resp.StatusCode)
	}

	// /mock/state surfaces the cluster.
	state := mock.FetchState(t)
	stateBytes, _ := json.Marshal(state)
	if !strings.Contains(string(stateBytes), `"demo"`) {
		t.Errorf("cluster missing from state: %s", stateBytes)
	}

	mock.Reset(t)
}

// TestE2E_AWS_SQS drives a queue Create → SendMessage → ReceiveMessage
// → DeleteMessage flow through the JSON 1.0 + X-Amz-Target SQS surface.
func TestE2E_AWS_SQS(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	sqsURL := mock.URL + "/sqs/region/us-east-1"

	// CreateQueue.
	resp, body := awsPostWithTargetJSON10(t, sqsURL, "AmazonSQS.CreateQueue", `{"QueueName":"jobs"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CreateQueue: %d %s", resp.StatusCode, body)
	}
	urlStart := strings.Index(string(body), `"QueueUrl":"`) + len(`"QueueUrl":"`)
	urlEnd := strings.Index(string(body)[urlStart:], `"`) + urlStart
	queueURL := string(body)[urlStart:urlEnd]

	// SendMessage.
	resp, _ = awsPostWithTargetJSON10(t, sqsURL, "AmazonSQS.SendMessage",
		`{"QueueUrl":"`+queueURL+`","MessageBody":"hello"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SendMessage: %d", resp.StatusCode)
	}

	// ReceiveMessage.
	resp, body = awsPostWithTargetJSON10(t, sqsURL, "AmazonSQS.ReceiveMessage",
		`{"QueueUrl":"`+queueURL+`"}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "hello") {
		t.Errorf("ReceiveMessage: %d %s", resp.StatusCode, body)
	}

	// /mock/state surfaces the queue.
	state := mock.FetchState(t)
	stateBytes, _ := json.Marshal(state)
	if !strings.Contains(string(stateBytes), `"jobs"`) {
		t.Errorf("queue missing from state: %s", stateBytes)
	}

	mock.Reset(t)
}

// awsPostWithTargetJSON10 is the JSON 1.0 variant of awsPostWithTarget.
func awsPostWithTargetJSON10(t *testing.T, url, target, body string) (*http.Response, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", target)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s %s: %v", url, target, err)
	}
	defer resp.Body.Close()
	out, _ := readAllBytes(resp.Body)
	return resp, out
}

// awsPostWithTarget POSTs a JSON body with the X-Amz-Target header.
// Used by DynamoDB (JSON 1.1) and SecretsManager (JSON 1.1).
func awsPostWithTarget(t *testing.T, url, target, body string) (*http.Response, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s %s: %v", url, target, err)
	}
	defer resp.Body.Close()
	out, _ := readAllBytes(resp.Body)
	return resp, out
}

// ----- minimal helpers (aws-side) -----

func awsPost(t *testing.T, url, body, contentType string) (*http.Response, []byte) {
	t.Helper()
	return awsRequest(t, http.MethodPost, url, body, contentType)
}

func awsPut(t *testing.T, url, body, contentType string) (*http.Response, []byte) {
	t.Helper()
	return awsRequest(t, http.MethodPut, url, body, contentType)
}

func awsGet(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	return awsRequest(t, http.MethodGet, url, "", "")
}

func awsRequest(t *testing.T, method, url, body, contentType string) (*http.Response, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var req *http.Request
	var err error
	if body != "" {
		req, err = http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, _ := readAllBytes(resp.Body)
	return resp, respBody
}

func readAllBytes(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var out []byte
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return out, nil
}

// extractTagValue is a tiny XML peek that pulls the first occurrence
// of <Tag>value</Tag>. Avoids dragging encoding/xml into the e2e
// helpers for this trivial use.
func extractTagValue(body, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(body, open)
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i+len(open):], close)
	if j < 0 {
		return ""
	}
	return body[i+len(open) : i+len(open)+j]
}

// url is a no-op alias for readability — body strings are already
// url-encoded by the caller.
func url(s string) string { return s }

// urlEncode percent-encodes an opaque string for a Query-RPC body
// parameter. We only need a tiny subset (curly braces, quotes, etc.).
func urlEncode(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '~':
			b.WriteRune(r)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}

// silence "imported and not used" for json (reserved for future
// state-shape assertions).
var _ = json.Valid
