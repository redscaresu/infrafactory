# name is omitted, so the profile's name is unknown at plan.
resource "aws_iam_instance_profile" "web" {}

resource "aws_instance" "web" {
  ami                  = "ami-0al2023x8664"
  instance_type        = "t3.micro"
  iam_instance_profile = aws_iam_instance_profile.web.name
}
