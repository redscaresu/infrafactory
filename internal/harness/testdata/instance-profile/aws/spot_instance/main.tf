resource "aws_spot_instance_request" "web" {
  ami                  = "ami-0al2023x8664"
  instance_type        = "t3.micro"
  iam_instance_profile = "web-profile"
}
