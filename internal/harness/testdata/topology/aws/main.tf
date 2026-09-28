# Capture HCL for the deriveTopologyAWS fixtures. The defaults are the
# positive case (fakeaws examples/working/web_step_one); each variable
# flips one reachability condition. capture.sh applies every variant
# against fakeaws and writes the trimmed /mock/state beside this file.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.100.0"
    }
  }
}

provider "aws" {
  region              = "us-east-1"
  allowed_account_ids = ["000000000000"]
}

variable "attach_open" {
  type    = bool
  default = true
}

variable "igw_route" {
  type    = bool
  default = true
}

variable "associate" {
  type    = bool
  default = true
}

variable "public_ip" {
  type    = bool
  default = true
}

variable "ingress" {
  type = list(object({
    from_port        = number
    to_port          = number
    protocol         = string
    cidr_blocks      = list(string)
    ipv6_cidr_blocks = list(string)
  }))
  default = [{
    from_port        = 80
    to_port          = 80
    protocol         = "tcp"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = []
  }]
}

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = "us-east-1a"
  map_public_ip_on_launch = true
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
}

resource "aws_route" "internet" {
  count                  = var.igw_route ? 1 : 0
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.main.id
}

resource "aws_route_table_association" "public" {
  count          = var.associate ? 1 : 0
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

# Always attached; egress only.
resource "aws_security_group" "base" {
  name   = "base"
  vpc_id = aws_vpc.main.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# Carries var.ingress; attached only when var.attach_open.
resource "aws_security_group" "open" {
  name   = "open"
  vpc_id = aws_vpc.main.id

  dynamic "ingress" {
    for_each = var.ingress
    content {
      from_port        = ingress.value.from_port
      to_port          = ingress.value.to_port
      protocol         = ingress.value.protocol
      cidr_blocks      = ingress.value.cidr_blocks
      ipv6_cidr_blocks = ingress.value.ipv6_cidr_blocks
    }
  }
}

resource "aws_instance" "web" {
  ami                         = "ami-0al2023x8664"
  instance_type               = "t3.micro"
  subnet_id                   = aws_subnet.public.id
  vpc_security_group_ids      = var.attach_open ? [aws_security_group.base.id, aws_security_group.open.id] : [aws_security_group.base.id]
  associate_public_ip_address = var.public_ip
}
