# AnyHost Terraform Module for AWS

Deploy AnyHost on AWS using ECS Fargate with Application Load Balancer.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         VPC                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Public Subnets                          │   │
│  │  ┌─────────────────────────────────────────────┐    │   │
│  │  │     Application Load Balancer               │    │   │
│  │  │     (HTTPS:443, HTTP:80 redirect)          │    │   │
│  │  └─────────────────┬───────────────────────────┘    │   │
│  └────────────────────┼────────────────────────────────┘   │
│                       │                                     │
│  ┌────────────────────▼────────────────────────────────┐   │
│  │              Private Subnets                         │   │
│  │  ┌─────────────────────────────────────────────┐    │   │
│  │  │         ECS Fargate Tasks                    │    │   │
│  │  │         (AnyHost Containers)                 │    │   │
│  │  └─────────────────┬───────────────────────────┘    │   │
│  │                    │                                 │   │
│  │  ┌─────────────────▼───────────────────────────┐    │   │
│  │  │              EFS                             │    │   │
│  │  │         (Persistent Storage)                 │    │   │
│  │  └─────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

- AWS Account with appropriate permissions
- Terraform >= 1.0
- VPC with public and private subnets
- ACM certificate for your domain
- Route53 hosted zone (recommended)

## Usage

### Basic Example

```hcl
module "anyhost" {
  source = "./deploy/terraform/aws"

  vpc_id             = "vpc-12345678"
  public_subnet_ids  = ["subnet-pub1", "subnet-pub2"]
  private_subnet_ids = ["subnet-priv1", "subnet-priv2"]
  domain             = "tunnel.example.com"
  certificate_arn    = "arn:aws:acm:us-east-1:123456789:certificate/xxx"
}

# Create Route53 record
resource "aws_route53_record" "tunnel" {
  zone_id = "Z1234567890"
  name    = "tunnel.example.com"
  type    = "A"

  alias {
    name                   = module.anyhost.alb_dns_name
    zone_id                = module.anyhost.alb_zone_id
    evaluate_target_health = true
  }
}

# Wildcard record for subdomains
resource "aws_route53_record" "tunnel_wildcard" {
  zone_id = "Z1234567890"
  name    = "*.tunnel.example.com"
  type    = "A"

  alias {
    name                   = module.anyhost.alb_dns_name
    zone_id                = module.anyhost.alb_zone_id
    evaluate_target_health = true
  }
}
```

### With Authentication Tokens

```hcl
# Create secret for auth tokens
resource "aws_secretsmanager_secret" "auth_tokens" {
  name = "anyhost-auth-tokens"
}

resource "aws_secretsmanager_secret_version" "auth_tokens" {
  secret_id = aws_secretsmanager_secret.auth_tokens.id
  secret_string = <<EOF
mytoken1:user1
mytoken2:user2
EOF
}

module "anyhost" {
  source = "./deploy/terraform/aws"

  vpc_id                = "vpc-12345678"
  public_subnet_ids     = ["subnet-pub1", "subnet-pub2"]
  private_subnet_ids    = ["subnet-priv1", "subnet-priv2"]
  domain                = "tunnel.example.com"
  certificate_arn       = "arn:aws:acm:us-east-1:123456789:certificate/xxx"
  auth_token_secret_arn = aws_secretsmanager_secret.auth_tokens.arn
}
```

### Production Configuration

```hcl
module "anyhost" {
  source = "./deploy/terraform/aws"

  vpc_id             = "vpc-12345678"
  public_subnet_ids  = ["subnet-pub1", "subnet-pub2"]
  private_subnet_ids = ["subnet-priv1", "subnet-priv2"]
  domain             = "tunnel.example.com"
  certificate_arn    = "arn:aws:acm:us-east-1:123456789:certificate/xxx"

  # Scaling
  cpu           = 512
  memory        = 1024
  desired_count = 2

  # Features
  enable_efs                 = true
  enable_container_insights  = true
  enable_deletion_protection = true

  # Cost optimization
  use_fargate_spot = false  # Set to true for non-critical workloads

  tags = {
    Environment = "production"
    Team        = "platform"
  }
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| vpc_id | VPC ID | string | - | yes |
| public_subnet_ids | Public subnet IDs for ALB | list(string) | - | yes |
| private_subnet_ids | Private subnet IDs for ECS | list(string) | - | yes |
| domain | Base domain for tunnels | string | - | yes |
| certificate_arn | ACM certificate ARN | string | - | yes |
| name_prefix | Resource name prefix | string | "anyhost" | no |
| image_repository | Docker image repo | string | "anyhost/gotunnel" | no |
| image_tag | Docker image tag | string | "latest" | no |
| cpu | ECS task CPU units | number | 256 | no |
| memory | ECS task memory (MB) | number | 512 | no |
| desired_count | Number of tasks | number | 1 | no |
| enable_efs | Enable EFS storage | bool | true | no |
| auth_token_secret_arn | Secrets Manager ARN | string | "" | no |

## Outputs

| Name | Description |
|------|-------------|
| alb_dns_name | ALB DNS name |
| alb_zone_id | ALB zone ID (for Route53) |
| tunnel_endpoint | WebSocket endpoint |
| dashboard_url | Dashboard URL |

## Cost Estimation

| Component | Estimated Monthly Cost |
|-----------|----------------------|
| ECS Fargate (256 CPU, 512 MB) | ~$10-15 |
| ALB | ~$20 |
| EFS | ~$0.30/GB |
| Data Transfer | Variable |
| **Total** | **~$35-50/month** |

*Use Fargate Spot to reduce costs by up to 70% for non-critical workloads.*

## Security Considerations

1. **Network**: Tasks run in private subnets, only accessible via ALB
2. **Encryption**: EFS encrypted at rest, HTTPS for all traffic
3. **Secrets**: Auth tokens stored in Secrets Manager
4. **IAM**: Least privilege roles for ECS tasks
