output "service_name" {
  value = aws_ecs_service.this.name
}

output "task_definition_family" {
  value = aws_ecs_task_definition.this.family
}

output "blue_target_group_arn" {
  value = aws_lb_target_group.blue.arn
}

output "green_target_group_arn" {
  value = aws_lb_target_group.green.arn
}

output "listener_rule_arn" {
  value = aws_lb_listener_rule.this.arn
}

output "cloudmap_dns_name" {
  description = "How other services on this Cloud Map namespace resolve this one, e.g. http://notifications.internal:8080"
  value       = "${var.service_name}.internal"
}
