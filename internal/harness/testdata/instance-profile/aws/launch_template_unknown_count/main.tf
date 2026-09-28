# The block's for_each is unknown at plan, so plan values cannot say
# whether the template carries a profile at all.
resource "aws_iam_instance_profile" "web" {}

resource "aws_launch_template" "web" {
  name          = "web"
  image_id      = "ami-0al2023x8664"
  instance_type = "t3.micro"

  dynamic "iam_instance_profile" {
    for_each = aws_iam_instance_profile.web.name == "" ? [] : ["web-profile"]
    content {
      name = iam_instance_profile.value
    }
  }
}
