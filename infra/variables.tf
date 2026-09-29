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
  description = "Initial image tag for the ECR repositories, used only until the first CI deploy overrides it via `aws ecs update-service`."
  type        = string
  default     = "initial"
}

variable "api_key_secret_value" {
  description = "Placeholder API key demonstrating Secrets Manager injection. Not a real credential."
  type        = string
  default     = "demo-secret-value"
  sensitive   = true
}

variable "github_owner" {
  description = "GitHub username"
  type        = string
}

variable "github_owner_id" {
  description = "Numeric GitHub owner ID. Get it with the cli command: gh api users/<owner> --jq .id (or orgs/<owner> for an organization)."
  type        = string
}

variable "github_repo" {
  description = "GitHub repository allowed to assume the CI role via OIDC"
  type        = string
}

variable "github_repo_id" {
  description = "Numeric GitHub repository ID. Get it with the cli command: gh api repos/<owner>/<repo> --jq .id."
  type        = string
}
