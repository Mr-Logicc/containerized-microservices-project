variable "project_name" {
  type = string
}

variable "service_name" {
  description = "Short name, e.g. \"auth\", \"orders\", \"notifications\". Used in resource names and as the Cloud Map DNS name."
  type        = string
}

variable "container_port" {
  type = number
}

variable "path_pattern" {
  description = "ALB listener rule path pattern this service owns, e.g. \"/api/orders/*\"."
  type        = string
}

variable "listener_priority" {
  description = "ALB listener rule priority. Must be unique across all services sharing the listener."
  type        = number
}

variable "cluster_arn" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "ecs_tasks_sg_id" {
  type = string
}

variable "alb_listener_arn" {
  type = string
}

variable "execution_role_arn" {
  type = string
}

variable "task_role_arn" {
  type = string
}

variable "bluegreen_infra_role_arn" {
  description = "IAM role ECS assumes to re-weight the ALB listener rule during a blue/green deployment."
  type        = string
}

variable "cloudmap_namespace_arn" {
  description = "ARN of the Cloud Map HTTP namespace used for Service Connect (not a private DNS namespace ID -- Service Connect needs the ARN)."
  type        = string
}

variable "ecr_repository_url" {
  type = string
}

variable "image_tag" {
  type    = string
  default = "initial"
}

variable "environment_variables" {
  description = "Plain (non-secret) environment variables for the app container."
  type        = map(string)
  default     = {}
}

variable "secrets" {
  description = "Map of env var name -> Secrets Manager secret ARN, injected into the app container."
  type        = map(string)
  default     = {}
}

variable "task_cpu" {
  description = "Total task CPU units, shared between the app container and the ADOT sidecar."
  type        = number
  default     = 256
}

variable "task_memory" {
  description = "Total task memory (MiB), shared between the app container and the ADOT sidecar."
  type        = number
  default     = 1024
}

variable "otel_collector_image" {
  type    = string
  default = "public.ecr.aws/aws-observability/aws-otel-collector:latest"
}

variable "log_retention_days" {
  type    = number
  default = 7
}
