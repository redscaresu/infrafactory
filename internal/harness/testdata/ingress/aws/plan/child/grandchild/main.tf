# Depth two: the same pair, over IPv6.

resource "aws_security_group" "open" {
  name = "grandchild-open"

  ingress {
    from_port        = 22
    to_port          = 22
    protocol         = "tcp"
    ipv6_cidr_blocks = ["::/0"]
  }
}

resource "aws_security_group" "bare" {
  name = "grandchild-bare"
}
