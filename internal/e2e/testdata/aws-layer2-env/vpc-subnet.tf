resource "aws_vpc" "main" {
  cidr_block = "10.70.0.0/16"
}

resource "aws_subnet" "a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.70.1.0/24"
  availability_zone = "us-east-1a"
}
