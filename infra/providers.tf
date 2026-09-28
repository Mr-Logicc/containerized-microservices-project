terraform {
  required_version = ">= 1.9.0"

  required_providers {
    aws = {
      source = "hashicorp/aws"
      # Native ECS blue/green (deployment_configuration.strategy,
      # load_balancer.advanced_configuration) requires >= 6.20.0.
      version = ">= 6.20.0, < 7.0.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project   = var.project_name
      ManagedBy = "terraform"
    }
  }
}

data "aws_region" "current" {}
