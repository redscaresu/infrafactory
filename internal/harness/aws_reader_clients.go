package harness

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// The only SDK clients the account read builds: EC2 for the preflight and
// STS for the identity check. Every other read is ReadAWSAccount's raw
// replay through a wall. Each sends only through its wall, and the SDK
// never retries, because the account read owns retry. TestAWSReadSeamUsesOnlyTheWalledClients
// holds every options func here to setting o.HTTPClient to its wall.

func newAWSReadEC2Client(cfg aws.Config, wall *AWSReadWall) (*ec2.Client, error) {
	if err := checkAWSReadWall(wall, "ec2"); err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.HTTPClient = wall
		o.Retryer = aws.NopRetryer{}
	}), nil
}

func newAWSReadSTSClient(cfg aws.Config, wall *AWSReadWall) (*sts.Client, error) {
	if err := checkAWSReadWall(wall, "sts"); err != nil {
		return nil, err
	}
	return sts.NewFromConfig(cfg, func(o *sts.Options) {
		o.HTTPClient = wall
		o.Retryer = aws.NopRetryer{}
	}), nil
}

func checkAWSReadWall(wall *AWSReadWall, service string) error {
	if wall == nil {
		return fmt.Errorf("aws read %s client: needs a read wall, and none was given", service)
	}
	if wall.service != service {
		return fmt.Errorf("aws read %s client: the wall is bound to %s", service, wall.service)
	}
	return nil
}
