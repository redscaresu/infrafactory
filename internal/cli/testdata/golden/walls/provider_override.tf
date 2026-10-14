provider "aws" {
  access_key              = "infrafactory-rehearsal-fake-key"
  secret_key              = "infrafactory-rehearsal-fake-secret"
  skip_metadata_api_check = true
  endpoints {
    ec2   = "http://127.0.0.1:4566/ec2/region/eu-west-1"
    elbv2 = "http://127.0.0.1:9"
    s3    = "http://127.0.0.1:9"
    sts   = "http://127.0.0.1:4566/sts"
  }
}

provider "aws" {
  alias                   = "east"
  access_key              = "infrafactory-rehearsal-fake-key"
  secret_key              = "infrafactory-rehearsal-fake-secret"
  skip_metadata_api_check = true
  endpoints {
    ec2   = "http://127.0.0.1:4566/ec2/region/eu-west-1"
    elbv2 = "http://127.0.0.1:9"
    s3    = "http://127.0.0.1:9"
    sts   = "http://127.0.0.1:4566/sts"
  }
}

provider "aws" {
  alias                   = "west"
  access_key              = "infrafactory-rehearsal-fake-key"
  secret_key              = "infrafactory-rehearsal-fake-secret"
  skip_metadata_api_check = true
  endpoints {
    ec2   = "http://127.0.0.1:4566/ec2/region/eu-west-1"
    elbv2 = "http://127.0.0.1:9"
    s3    = "http://127.0.0.1:9"
    sts   = "http://127.0.0.1:4566/sts"
  }
}
