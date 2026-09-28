# A local child module, called with count so its address carries an
# instance key. open is denied at depth one; bare declares no ingress
# and passes only if the policy finds its configuration here.

resource "aws_security_group" "open" {
  name = "child-open"

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "bare" {
  name = "child-bare"
}

module "grandchild" {
  source = "./grandchild"
}
