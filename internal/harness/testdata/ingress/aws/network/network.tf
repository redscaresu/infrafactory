# Lines 19-41 of .infrafactory/runs/aws-instance/20260607T064123Z/generated/network.tf,
# the security group a 2026-06-07 aws-instance run generated. Verbatim:
# capture.sh derives the 0.0.0.0/0 variant from it.

resource "aws_security_group" "app" {
  name        = "aws-instance-sg"
  description = "Security group for app-server EC2 instance"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/16"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "aws-instance-sg"
  }
}