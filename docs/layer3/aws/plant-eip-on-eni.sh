#!/usr/bin/env bash
# An elastic IP on a standalone network interface, as the admin role: the one case that makes reap
# send DisassociateAddress (terminating an instance drops its elastic IP instead). Run apart from
# plant-leaks.sh. Writes every id to ./planted-eip.env in the directory it is run from.
set -euo pipefail
export AWS_PROFILE=infrafactory-admin AWS_REGION="${REGION:-us-east-1}"
out=planted-eip.env; : > "$out"
rec() { echo "$1=$2" >> "$out"; echo "$1=$2"; }
q() { aws ec2 "$@" --output text; }

VPC=$(q create-vpc --cidr-block 10.98.0.0/16 --query Vpc.VpcId); rec VPC "$VPC"
SUBNET=$(q create-subnet --vpc-id "$VPC" --cidr-block 10.98.1.0/24 --query Subnet.SubnetId); rec SUBNET "$SUBNET"
# An elastic IP can only be associated in a VPC with an internet gateway.
IGW=$(q create-internet-gateway --query InternetGateway.InternetGatewayId); rec IGW "$IGW"
aws ec2 attach-internet-gateway --internet-gateway-id "$IGW" --vpc-id "$VPC"
ENI=$(q create-network-interface --subnet-id "$SUBNET" --query NetworkInterface.NetworkInterfaceId); rec ENI "$ENI"
EIP=$(q allocate-address --domain vpc --query AllocationId); rec EIP "$EIP"
ASSOC=$(q associate-address --allocation-id "$EIP" --network-interface-id "$ENI" --query AssociationId); rec ASSOC "$ASSOC"
echo "# planted at $(date -u +%FT%TZ)" | tee -a "$out"
