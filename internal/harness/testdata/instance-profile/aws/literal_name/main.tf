resource "aws_instance" "web" {
  ami                  = "ami-0al2023x8664"
  instance_type        = "t3.micro"
  iam_instance_profile = "web-profile"
}
