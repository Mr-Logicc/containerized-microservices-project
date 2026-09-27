variable "aws_region" {
  description = "AWS region everything is deployed into."
  type        = string
  default     = "us-east-1"
}

variable "project_name" {
  description = "Short name used as a prefix for resource names and tags."
  type        = string
  default     = "demo"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "container_image_tag" {
  description = "Image tag ECR repos are seeded with before the very first deploy. GitHub Actions overrides the running tag afterwards via `aws ecs update-service`, so this only matters for the first `terraform apply`, before any image has been pushed."
  type        = string
  default     = "initial"
}

variable "api_key_secret_value" {
  description = "Fake API key / DB password used only to demonstrate the Secrets Manager injection pattern. Not a real credential"
  type        = string
  default     = "demo-secret-value"
  sensitive   = true
}

variable "github_repo" {
  description = "GitHub repo allowed to assume the CI role via OIDC, in \"owner/repo\" form (e.g. \"amir/microservices-project\"). Required -- there's no safe default, an open trust policy would let any GitHub repo assume this role."
  type        = string
}
