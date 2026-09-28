# Capture HCL for the default_deny_ingress plan fixtures. capture.sh
# plans this directory against fakeaws and writes the trimmed plan to
# plan.json. A resource the policy must deny is named open*, and no
# other is: aws_default_deny_ingress_test.go checks that set exactly.

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

# Rules attach to a literal group id so no helper group joins the walk.
locals {
  group_id = "sg-0123456789abcdef0"

  # Every public opening, written once and planned in each of the four
  # ingress shapes. The provider rewrites "all" to "-1" and "6" to "tcp"
  # on the inline and aws_security_group_rule shapes, but not on
  # aws_vpc_security_group_ingress_rule.
  open = {
    ssh_ipv4   = { protocol = "tcp", from = 22, to = 22, cidrs = ["0.0.0.0/0"] }
    ssh_ipv6   = { protocol = "tcp", from = 22, to = 22, cidrs = ["::/0"] }
    ssh_host   = { protocol = "tcp", from = 22, to = 22, cidrs = ["203.0.113.7/32"] }
    ssh_halves = { protocol = "tcp", from = 22, to = 22, cidrs = ["0.0.0.0/1", "128.0.0.0/1"] }
    ssh_six    = { protocol = "6", from = 22, to = 22, cidrs = ["0.0.0.0/0"] }
    all_minus1 = { protocol = "-1", from = 0, to = 0, cidrs = ["0.0.0.0/0"] }
    all_word   = { protocol = "all", from = 0, to = 0, cidrs = ["0.0.0.0/0"] }
    ports_0_0  = { protocol = "tcp", from = 0, to = 0, cidrs = ["0.0.0.0/0"] }
    tcp_full   = { protocol = "tcp", from = 0, to = 65535, cidrs = ["0.0.0.0/0"] }
  }

  # aws_vpc_security_group_ingress_rule takes one source per rule, so
  # ssh_halves is two rules there.
  open_per_cidr = merge([
    for k, v in local.open : { for i, c in v.cidrs : "${k}_${i}" => merge(v, { cidr = c }) }
  ]...)
}

resource "aws_vpc" "open" {
  for_each   = local.open
  cidr_block = "10.0.0.0/16"
}

resource "aws_security_group" "open" {
  for_each = local.open
  name     = "open-${each.key}"

  ingress {
    from_port        = each.value.from
    to_port          = each.value.to
    protocol         = each.value.protocol
    cidr_blocks      = [for c in each.value.cidrs : c if !strcontains(c, ":")]
    ipv6_cidr_blocks = [for c in each.value.cidrs : c if strcontains(c, ":")]
  }
}

resource "aws_default_security_group" "open" {
  for_each = local.open
  vpc_id   = aws_vpc.open[each.key].id

  ingress {
    from_port        = each.value.from
    to_port          = each.value.to
    protocol         = each.value.protocol
    cidr_blocks      = [for c in each.value.cidrs : c if !strcontains(c, ":")]
    ipv6_cidr_blocks = [for c in each.value.cidrs : c if strcontains(c, ":")]
  }
}

resource "aws_security_group_rule" "open" {
  for_each          = local.open
  type              = "ingress"
  security_group_id = local.group_id
  from_port         = each.value.from
  to_port           = each.value.to
  protocol          = each.value.protocol
  cidr_blocks       = [for c in each.value.cidrs : c if !strcontains(c, ":")]
  ipv6_cidr_blocks  = [for c in each.value.cidrs : c if strcontains(c, ":")]
}

resource "aws_vpc_security_group_ingress_rule" "open" {
  for_each          = local.open_per_cidr
  security_group_id = local.group_id
  ip_protocol       = each.value.protocol
  from_port         = contains(["-1", "all"], each.value.protocol) ? null : each.value.from
  to_port           = contains(["-1", "all"], each.value.protocol) ? null : each.value.to
  cidr_ipv4         = strcontains(each.value.cidr, ":") ? null : each.value.cidr
  cidr_ipv6         = strcontains(each.value.cidr, ":") ? each.value.cidr : null
}

# A prefix list is refused on any port, here 443.
resource "aws_vpc" "open_prefix_list" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_security_group" "open_prefix_list" {
  name = "open-prefix-list"

  ingress {
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    prefix_list_ids = ["pl-12345678"]
  }
}

resource "aws_default_security_group" "open_prefix_list" {
  vpc_id = aws_vpc.open_prefix_list.id

  ingress {
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    prefix_list_ids = ["pl-12345678"]
  }
}

resource "aws_security_group_rule" "open_prefix_list" {
  type              = "ingress"
  security_group_id = local.group_id
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  prefix_list_ids   = ["pl-12345678"]
}

resource "aws_vpc_security_group_ingress_rule" "open_prefix_list" {
  security_group_id = local.group_id
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  prefix_list_id    = "pl-12345678"
}

module "child" {
  source = "./child"
  count  = 1
}
