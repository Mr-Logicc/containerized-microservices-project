resource "aws_service_discovery_http_namespace" "internal" {
  name        = "internal"
  description = "Cloud Map namespace backing Service Connect for auth/orders/notifications"
}
