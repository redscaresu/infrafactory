---
kind: code
status: ready
epic: aws-layer3-gate
depends_on: []
touches: ["go.mod", "go.sum", "internal/harness/aws_ec2_reads.go (new)", "internal/harness/aws_ec2_reads_test.go (new)", "docs/stories/aws-ec2-reads.md (delete)"]
risk: high
---

# A sealed EC2 client reads an instance's user data and a resolved AMI's root device, and the state names the one instance to read

New file internal/harness/aws_ec2_reads.go. It adds github.com/aws/aws-sdk-go-v2/service/ec2 (go.mod:9-10 has only ssm and sts). The client is built the same way ResolveAWSAMIFromSSM builds its SSM client (aws_ami.go:30-49): sealedAWSConfig (aws_client.go:50), BaseEndpoint https://ec2.<region>.amazonaws.com unless an endpoint is passed, and a required doer.
- AWSInstanceUserData(ctx, env, doer, endpoint, instanceID) calls DescribeInstanceAttribute for userData and returns the base64-decoded bytes. An absent or empty value is an error, and so are malformed base64 and an id not matching ^i-[0-9a-f]+$.
- AWSStateInstanceID(workDir) returns the one managed aws_instance id in terraform-live.tfstate, read as UnplacedAWSResources reads it (aws_placement.go:31). Zero instances, or more than one, is an error.
- AWSAMIRoot(ctx, env, doer, endpoint, amiID) calls DescribeImages for exactly that one id. It returns the root mapping, meaning the one whose DeviceName equals RootDeviceName, as AWSAMIRoot{SizeGiB, VolumeType, DeleteOnTermination}. It is an error if there is not exactly one image, if no root mapping is found, or if the root is not EBS.

Nothing compares here; the callers do. The state's user_data SHA1 is never read. Tests use aws-seal's capturing doer and its loopback-only doer (aws_client.go:66-73).

**Done when:**
- Capturing-doer test for AWSInstanceUserData and for AWSAMIRoot: with no endpoint argument, each request goes to https://ec2.us-east-1.amazonaws.com/ with its action and parameters, signed with the sealed key. Planted AWS_ENDPOINT_URL_EC2, AWS_ENDPOINT_URL and AWS_PROFILE are ignored, and a trap loopback server counts zero requests
- AWSInstanceUserData against a loopback fake EC2: base64 of the golden web-step-one script returns exactly those bytes. A missing value, an empty value, non-base64 and a 403 UnauthorizedOperation each return an error naming the cause
- AWSAMIRoot against the fake: an 8 GiB gp3 root with DeleteOnTermination true is returned as such. Zero images, two images, a root DeviceName with no matching mapping, and an instance-store root each return an error
- AWSInstanceUserData and AWSAMIRoot each refuse, with zero requests, a nil doer, a malformed region, the instance id 'i-../x' and the AMI id 'ami-../x'
- AWSStateInstanceID returns the id for one aws_instance. It errors on zero instances, two instances, a state holding only a data source, and an unreadable state
- aws_sdk_audit_test.go passes unmodified, and the govulncheck CI job passes
