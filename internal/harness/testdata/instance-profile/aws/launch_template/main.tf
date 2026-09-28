resource "aws_launch_template" "web" {
  name          = "web"
  image_id      = "ami-0al2023x8664"
  instance_type = "t3.micro"

  iam_instance_profile {
    name = "web-profile"
  }
}
