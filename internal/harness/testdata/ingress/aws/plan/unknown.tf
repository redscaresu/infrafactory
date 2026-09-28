# Openings whose source, ports or protocol are unknown until apply. An
# unknown source counts as public, and an unknown port or protocol as
# admitting 22, so every one of these is denied.

resource "aws_eip" "x" {
  domain = "vpc"
}

locals {
  # Values that are 22, 80 and "tcp" once applied, and unknown at plan.
  # The branches differ because OpenTofu folds `unknown ? 80 : 80` to 80.
  later_22  = aws_eip.x.public_ip != "" ? 22 : 23
  later_80  = aws_eip.x.public_ip != "" ? 80 : 81
  later_tcp = aws_eip.x.public_ip != "" ? "tcp" : "udp"

  open_unknown = {
    eip_source = { protocol = "tcp", from = 22, to = 22, cidr = "${aws_eip.x.public_ip}/32" }
    from_port  = { protocol = "tcp", from = local.later_80, to = 80, cidr = "0.0.0.0/0" }
    to_port    = { protocol = "tcp", from = 80, to = local.later_80, cidr = "0.0.0.0/0" }
    protocol   = { protocol = local.later_tcp, from = 80, to = 80, cidr = "0.0.0.0/0" }
  }
}

resource "aws_vpc" "open_unknown" {
  for_each   = local.open_unknown
  cidr_block = "10.0.0.0/16"
}

resource "aws_security_group" "open_unknown" {
  for_each = local.open_unknown
  name     = "open-unknown-${each.key}"

  ingress {
    from_port   = each.value.from
    to_port     = each.value.to
    protocol    = each.value.protocol
    cidr_blocks = [each.value.cidr]
  }
}

resource "aws_default_security_group" "open_unknown" {
  for_each = local.open_unknown
  vpc_id   = aws_vpc.open_unknown[each.key].id

  ingress {
    from_port   = each.value.from
    to_port     = each.value.to
    protocol    = each.value.protocol
    cidr_blocks = [each.value.cidr]
  }
}

resource "aws_security_group_rule" "open_unknown" {
  for_each          = local.open_unknown
  type              = "ingress"
  security_group_id = local.group_id
  from_port         = each.value.from
  to_port           = each.value.to
  protocol          = each.value.protocol
  cidr_blocks       = [each.value.cidr]
}

resource "aws_vpc_security_group_ingress_rule" "open_unknown" {
  for_each          = local.open_unknown
  security_group_id = local.group_id
  ip_protocol       = each.value.protocol
  from_port         = each.value.from
  to_port           = each.value.to
  cidr_ipv4         = each.value.cidr
}

resource "aws_security_group" "open_dynamic" {
  name = "open-dynamic"

  dynamic "ingress" {
    for_each = [aws_eip.x.public_ip]
    content {
      from_port   = 22
      to_port     = 22
      protocol    = "tcp"
      cidr_blocks = ["${ingress.value}/32"]
    }
  }
}

# for_each itself is unknown: the plan holds one placeholder rule whose
# every field is unknown, private literal or not.
resource "aws_security_group" "open_dynamic_collection" {
  name = "open-dynamic-collection"

  dynamic "ingress" {
    for_each = local.later_22 == 22 ? [22] : []
    content {
      from_port   = ingress.value
      to_port     = ingress.value
      protocol    = "tcp"
      cidr_blocks = ["10.0.0.0/8"]
    }
  }
}

resource "aws_security_group" "open_attribute" {
  name = "open-attribute"
  ingress = [{
    from_port        = 22
    to_port          = 22
    protocol         = "tcp"
    cidr_blocks      = ["${aws_eip.x.public_ip}/32"]
    ipv6_cidr_blocks = []
    prefix_list_ids  = []
    security_groups  = []
    self             = false
    description      = ""
  }]
}

# The list itself is unknown, so the plan holds `ingress` as one unknown
# value with no rules in it, private literal or not.
resource "aws_security_group" "open_attribute_collection" {
  name = "open-attribute-collection"
  ingress = local.later_22 == 22 ? [{
    from_port        = 22
    to_port          = 22
    protocol         = "tcp"
    cidr_blocks      = ["10.0.0.0/8"]
    ipv6_cidr_blocks = []
    prefix_list_ids  = []
    security_groups  = []
    self             = false
    description      = ""
  }] : []
}

# type is unknown until apply, so the rule may be ingress.
resource "aws_security_group_rule" "open_unknown_type" {
  type              = aws_eip.x.public_ip != "" ? "ingress" : "egress"
  security_group_id = local.group_id
  from_port         = 22
  to_port           = 22
  protocol          = "tcp"
  cidr_blocks       = ["0.0.0.0/0"]
}
