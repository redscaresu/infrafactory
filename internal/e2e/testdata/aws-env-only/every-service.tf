# One resource per service fakeaws serves without Docker. No provider
# block: generation writes it, default_tags with the run id included, and
# every endpoint comes from cloudEnv.

resource "aws_vpc" "main" {
  cidr_block = "10.80.0.0/16"
}

resource "aws_subnet" "a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.80.1.0/24"
  availability_zone = "us-east-1a"
}

resource "aws_subnet" "b" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.80.2.0/24"
  availability_zone = "us-east-1b"
}

resource "aws_iam_role" "app" {
  name = "env-only-app"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_sqs_queue" "jobs" {
  name = "env-only-jobs"
}

resource "aws_dynamodb_table" "items" {
  name         = "env-only-items"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }
}

# No description: fakeaws does not return one yet
# (docs/stories/fakeaws-kms-key-description.md).
resource "aws_kms_key" "app" {
  deletion_window_in_days = 7
}

resource "aws_route53_zone" "main" {
  name = "env-only.example.invalid."
}

resource "aws_secretsmanager_secret" "app" {
  name                    = "env-only-secret"
  recovery_window_in_days = 0
}

resource "aws_db_subnet_group" "main" {
  name       = "env-only-db"
  subnet_ids = [aws_subnet.a.id, aws_subnet.b.id]
}

resource "aws_db_parameter_group" "main" {
  name   = "env-only-pg15"
  family = "postgres15"
}

resource "aws_eks_cluster" "main" {
  name     = "env-only-cluster"
  role_arn = aws_iam_role.app.arn
  version  = "1.29"

  vpc_config {
    subnet_ids = [aws_subnet.a.id, aws_subnet.b.id]
  }
}
