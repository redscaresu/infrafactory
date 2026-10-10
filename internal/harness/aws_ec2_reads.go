package harness

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// awsInstanceIDRe is an EC2 instance id. The id goes into an EC2 request,
// so nothing else does.
var awsInstanceIDRe = regexp.MustCompile(`^i-[0-9a-f]+$`)

// AWSAMIRoot is an AMI's root EBS mapping as DescribeImages reports it.
type AWSAMIRoot struct {
	DeviceName          string
	SizeGiB             int32
	VolumeType          string
	DeleteOnTermination bool
}

// newAWSEC2Client builds an EC2 client as newAWSSSMClient builds its
// SSM one: the sealed env's static key and region, never the SDK default
// chain. endpoint "" means real EC2 in that region. doer is required, so
// no caller reaches EC2 through a client it did not choose.
func newAWSEC2Client(env map[string]string, doer ec2.HTTPClient, endpoint string) (*ec2.Client, error) {
	if doer == nil {
		return nil, errors.New("aws ec2 client: needs an HTTP client, and none was given")
	}
	cfg, err := sealedAWSConfig(env, "aws ec2 client")
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		endpoint = "https://ec2." + cfg.Region + ".amazonaws.com"
	}
	return ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.HTTPClient = doer
	}), nil
}

// AWSInstanceUserData returns instanceID's user data as EC2 holds it,
// base64-decoded. An absent or empty value is an error: the caller
// compares these bytes, and nothing must compare equal to nothing.
func AWSInstanceUserData(ctx context.Context, env map[string]string, doer ec2.HTTPClient, endpoint, instanceID string) ([]byte, error) {
	if !awsInstanceIDRe.MatchString(instanceID) {
		return nil, fmt.Errorf("aws ec2: %q is not an instance id", instanceID)
	}
	client, err := newAWSEC2Client(env, doer, endpoint)
	if err != nil {
		return nil, err
	}

	out, err := client.DescribeInstanceAttribute(ctx, &ec2.DescribeInstanceAttributeInput{
		InstanceId: aws.String(instanceID),
		Attribute:  types.InstanceAttributeNameUserData,
	})
	if err != nil {
		return nil, fmt.Errorf("aws ec2: DescribeInstanceAttribute userData for %s failed: %w", instanceID, err)
	}
	if out.UserData == nil || out.UserData.Value == nil {
		return nil, fmt.Errorf("aws ec2: instance %s reports no user data", instanceID)
	}
	if *out.UserData.Value == "" {
		return nil, fmt.Errorf("aws ec2: instance %s reports empty user data", instanceID)
	}
	decoded, err := base64.StdEncoding.DecodeString(*out.UserData.Value)
	if err != nil {
		return nil, fmt.Errorf("aws ec2: instance %s's user data is not base64: %w", instanceID, err)
	}
	return decoded, nil
}

// AWSStateInstanceID returns the id of the one managed aws_instance in
// workDir's live state. A data source names something the run looked up,
// not something it launched, so it never counts.
func AWSStateInstanceID(workDir string) (string, error) {
	state, err := loadLiveTerraformState(filepath.Join(workDir, LiveStateFilename))
	if err != nil {
		return "", err
	}
	var ids []string
	for _, resource := range state.Resources {
		if resource.Type != "aws_instance" || !managedCloudResource(resource) {
			continue
		}
		for _, instance := range resource.Instances {
			id, _ := instance.Attributes["id"].(string)
			ids = append(ids, id)
		}
	}
	if len(ids) != 1 {
		return "", fmt.Errorf("the live state holds %d managed aws_instance, want exactly one", len(ids))
	}
	if ids[0] == "" {
		return "", errors.New("the live state's aws_instance has no id")
	}
	return ids[0], nil
}

// DescribeAWSAMIRoot returns amiID's root mapping: the one whose
// DeviceName is the image's RootDeviceName. The root must be EBS.
func DescribeAWSAMIRoot(ctx context.Context, env map[string]string, doer ec2.HTTPClient, endpoint, amiID string) (AWSAMIRoot, error) {
	if !awsAMIIDRe.MatchString(amiID) {
		return AWSAMIRoot{}, fmt.Errorf("aws ec2: %q is not an AMI id", amiID)
	}
	client, err := newAWSEC2Client(env, doer, endpoint)
	if err != nil {
		return AWSAMIRoot{}, err
	}

	out, err := client.DescribeImages(ctx, &ec2.DescribeImagesInput{ImageIds: []string{amiID}})
	if err != nil {
		return AWSAMIRoot{}, fmt.Errorf("aws ec2: DescribeImages %s failed: %w", amiID, err)
	}
	if len(out.Images) != 1 {
		return AWSAMIRoot{}, fmt.Errorf("aws ec2: DescribeImages %s returned %d images, want exactly one", amiID, len(out.Images))
	}
	image := out.Images[0]
	if got := aws.ToString(image.ImageId); got != amiID {
		return AWSAMIRoot{}, fmt.Errorf("aws ec2: DescribeImages %s returned image %q", amiID, got)
	}
	if image.RootDeviceType != types.DeviceTypeEbs {
		return AWSAMIRoot{}, fmt.Errorf("aws ec2: image %s's root device type is %q, not ebs", amiID, image.RootDeviceType)
	}
	root := aws.ToString(image.RootDeviceName)
	for _, mapping := range image.BlockDeviceMappings {
		if root == "" || aws.ToString(mapping.DeviceName) != root || mapping.Ebs == nil {
			continue
		}
		return AWSAMIRoot{
			DeviceName:          root,
			SizeGiB:             aws.ToInt32(mapping.Ebs.VolumeSize),
			VolumeType:          string(mapping.Ebs.VolumeType),
			DeleteOnTermination: aws.ToBool(mapping.Ebs.DeleteOnTermination),
		}, nil
	}
	return AWSAMIRoot{}, fmt.Errorf("aws ec2: image %s has no EBS mapping for its root device %q", amiID, root)
}
