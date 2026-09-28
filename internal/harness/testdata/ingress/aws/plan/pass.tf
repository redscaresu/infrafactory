# What the policy must pass: SSH from a group, from itself or from a
# private CIDR, public HTTP, and all-traffic egress.

variable "vpc_cidr" {
  type    = string
  default = "10.1.0.0/16"
}

locals {
  pass = {
    web         = { protocol = "tcp", from = 80, to = 80, cidrs = ["0.0.0.0/0", "::/0"] }
    ssh_private = { protocol = "tcp", from = 22, to = 22, cidrs = ["10.0.0.0/16", "172.16.0.0/12", "192.168.1.0/24", "fd00::/8"] }
  }
  pass_per_cidr = merge([
    for k, v in local.pass : { for i, c in v.cidrs : "${k}_${i}" => merge(v, { cidr = c }) }
  ]...)
}

resource "aws_vpc" "literal" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_vpc" "var_default" {
  cidr_block = var.vpc_cidr
}

# Declares no ingress, so the plan holds `ingress` as one unknown value.
resource "aws_security_group" "peer" {
  name   = "peer"
  vpc_id = aws_vpc.literal.id
}

resource "aws_security_group" "pass" {
  for_each = local.pass
  name     = "pass-${each.key}"

  ingress {
    from_port        = each.value.from
    to_port          = each.value.to
    protocol         = each.value.protocol
    cidr_blocks      = [for c in each.value.cidrs : c if !strcontains(c, ":")]
    ipv6_cidr_blocks = [for c in each.value.cidrs : c if strcontains(c, ":")]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group_rule" "pass" {
  for_each          = local.pass
  type              = "ingress"
  security_group_id = local.group_id
  from_port         = each.value.from
  to_port           = each.value.to
  protocol          = each.value.protocol
  cidr_blocks       = [for c in each.value.cidrs : c if !strcontains(c, ":")]
  ipv6_cidr_blocks  = [for c in each.value.cidrs : c if strcontains(c, ":")]
}

resource "aws_vpc_security_group_ingress_rule" "pass" {
  for_each          = local.pass_per_cidr
  security_group_id = local.group_id
  ip_protocol       = each.value.protocol
  from_port         = each.value.from
  to_port           = each.value.to
  cidr_ipv4         = strcontains(each.value.cidr, ":") ? null : each.value.cidr
  cidr_ipv6         = strcontains(each.value.cidr, ":") ? each.value.cidr : null
}

resource "aws_security_group" "ssh_from_group" {
  name = "ssh-from-group"

  ingress {
    from_port       = 22
    to_port         = 22
    protocol        = "tcp"
    security_groups = [aws_security_group.peer.id]
  }

  ingress {
    from_port = 0
    to_port   = 0
    protocol  = "-1"
    self      = true
  }
}

resource "aws_default_security_group" "ssh_from_group" {
  vpc_id = aws_vpc.literal.id

  ingress {
    from_port       = 22
    to_port         = 22
    protocol        = "tcp"
    security_groups = [aws_security_group.peer.id]
  }

  ingress {
    from_port = 0
    to_port   = 0
    protocol  = "-1"
    self      = true
  }
}

resource "aws_security_group_rule" "ssh_from_group" {
  type                     = "ingress"
  security_group_id        = local.group_id
  from_port                = 22
  to_port                  = 22
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.peer.id
}

resource "aws_security_group_rule" "all_from_self" {
  type              = "ingress"
  security_group_id = local.group_id
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  self              = true
}

resource "aws_vpc_security_group_ingress_rule" "ssh_from_group" {
  security_group_id            = local.group_id
  ip_protocol                  = "tcp"
  from_port                    = 22
  to_port                      = 22
  referenced_security_group_id = aws_security_group.peer.id
}

# The VPC's CIDR is known at plan when it is a literal or a variable
# default, so the rule is judged as the private CIDR it is.
resource "aws_security_group" "ssh_from_vpc" {
  name = "ssh-from-vpc"

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.literal.cidr_block]
  }

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.var_default.cidr_block]
  }
}

resource "aws_security_group_rule" "ssh_from_vpc" {
  type              = "ingress"
  security_group_id = local.group_id
  from_port         = 22
  to_port           = 22
  protocol          = "tcp"
  cidr_blocks       = [aws_vpc.var_default.cidr_block]
}

# Egress is never walked, in any of its three spellings.
resource "aws_security_group_rule" "egress_all" {
  type              = "egress"
  security_group_id = local.group_id
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = local.group_id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

# Public ping. ICMP takes no ports here, so they plan as null, which is
# not unknown.
resource "aws_vpc_security_group_ingress_rule" "ping" {
  security_group_id = local.group_id
  ip_protocol       = "icmp"
  cidr_ipv4         = "0.0.0.0/0"
}
