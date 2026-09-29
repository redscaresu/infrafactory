#!/usr/bin/env bash
# Plant one leak per swept collection in the AWS Layer 3 scope, as the admin role, for the
# planted-leak proof (docs/operations.md § Layer 3 (AWS) › Re-running the planted-leak proof).
# Creates billable resources (a NAT gateway and a t3.micro among them): reap them straight after.
# Writes every id to ./planted.env in the directory it is run from.
set -euo pipefail
export AWS_PROFILE=infrafactory-admin AWS_REGION="${REGION:-us-east-1}"
out=planted.env; : > "$out"
rec() { echo "$1=$2" >> "$out"; echo "$1=$2"; }
q() { aws ec2 "$@" --output text; }

VPC=$(q create-vpc --cidr-block 10.99.0.0/16 --query Vpc.VpcId); rec VPC "$VPC"
SUBNET=$(q create-subnet --vpc-id "$VPC" --cidr-block 10.99.1.0/24 --query Subnet.SubnetId); rec SUBNET "$SUBNET"
AZ=$(q describe-subnets --subnet-ids "$SUBNET" --query 'Subnets[0].AvailabilityZone')
IGW=$(q create-internet-gateway --query InternetGateway.InternetGatewayId); rec IGW "$IGW"
aws ec2 attach-internet-gateway --internet-gateway-id "$IGW" --vpc-id "$VPC"
RT=$(q create-route-table --vpc-id "$VPC" --query RouteTable.RouteTableId); rec RT "$RT"
aws ec2 create-route --route-table-id "$RT" --destination-cidr-block 0.0.0.0/0 --gateway-id "$IGW" >/dev/null
aws ec2 associate-route-table --route-table-id "$RT" --subnet-id "$SUBNET" >/dev/null
SGA=$(q create-security-group --group-name leak-a --description leak-a --vpc-id "$VPC" --query GroupId); rec SGA "$SGA"
SGB=$(q create-security-group --group-name leak-b --description leak-b --vpc-id "$VPC" --query GroupId); rec SGB "$SGB"
aws ec2 authorize-security-group-ingress --group-id "$SGA" --protocol tcp --port 80 --source-group "$SGB" >/dev/null
KP=$(q create-key-pair --key-name leak-kp --query KeyPairId); rec KP "$KP"   # the private key is discarded
AMI=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 \
  --query Parameter.Value --output text)
I=$(q run-instances --image-id "$AMI" --instance-type t3.micro --subnet-id "$SUBNET" --security-group-ids "$SGA" \
  --query 'Instances[0].InstanceId'); rec INSTANCE "$I"
aws ec2 wait instance-running --instance-ids "$I"
EIP1=$(q allocate-address --domain vpc --query AllocationId); rec EIP1 "$EIP1"
aws ec2 associate-address --allocation-id "$EIP1" --instance-id "$I" >/dev/null
EIP2=$(q allocate-address --domain vpc --query AllocationId); rec EIP2 "$EIP2"
ENI=$(q create-network-interface --subnet-id "$SUBNET" --groups "$SGB" --query NetworkInterface.NetworkInterfaceId); rec ENI "$ENI"
VOL=$(q create-volume --availability-zone "$AZ" --size 1 --volume-type gp3 --query VolumeId); rec VOLUME "$VOL"
aws ec2 wait volume-available --volume-ids "$VOL"
SNAP=$(q create-snapshot --volume-id "$VOL" --query SnapshotId); rec SNAPSHOT "$SNAP"
aws ec2 wait snapshot-completed --snapshot-ids "$SNAP"
IMG=$(q register-image --name leak-img --architecture x86_64 --virtualization-type hvm --ena-support \
  --root-device-name /dev/xvda --block-device-mappings "DeviceName=/dev/xvda,Ebs={SnapshotId=$SNAP}" --query ImageId); rec IMAGE "$IMG"
aws ec2 disable-image --image-id "$IMG" >/dev/null
LT=$(q create-launch-template --launch-template-name leak-lt --launch-template-data '{"InstanceType":"t3.micro"}' \
  --query LaunchTemplate.LaunchTemplateId); rec LAUNCH_TEMPLATE "$LT"
EIP3=$(q allocate-address --domain vpc --query AllocationId); rec EIP3 "$EIP3"
NAT=$(q create-nat-gateway --subnet-id "$SUBNET" --allocation-id "$EIP3" --query NatGateway.NatGatewayId); rec NAT "$NAT"
aws ec2 wait nat-gateway-available --nat-gateway-ids "$NAT"
aws ssm put-parameter --name /leak/outside --type String --value planted >/dev/null; rec PARAM /leak/outside
echo "# planted at $(date -u +%FT%TZ)" | tee -a "$out"
