output "alb_dns_name" {
  description = "Public URL base -- try http://<this>/api/auth/health, /api/orders/health, /api/notifications/health"
  value       = aws_lb.this.dns_name
}

output "ecr_repository_urls" {
  value = { for name, repo in aws_ecr_repository.services : name => repo.repository_url }
}

output "github_actions_role_arn" {
  description = "Put this in the GitHub Actions workflow's role-to-assume input"
  value       = aws_iam_role.github_actions.arn
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.this.name
}

output "redis_endpoint" {
  value = aws_elasticache_cluster.redis.cache_nodes[0].address
}

output "frontend_url" {
  description = "Open this in a browser to use the console"
  value       = "https://${aws_cloudfront_distribution.frontend.domain_name}"
}

output "frontend_bucket_name" {
  value = aws_s3_bucket.frontend.id
}

output "cloudfront_distribution_id" {
  description = "Needed by GitHub Actions to invalidate the cache after each deploy"
  value       = aws_cloudfront_distribution.frontend.id
}
