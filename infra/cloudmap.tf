# HTTP namespace (not a private DNS namespace) backing ECS Service Connect.
# Classic Cloud Map service_registries is not supported on BLUE_GREEN
# services; Service Connect is the supported discovery mechanism instead.
resource "aws_service_discovery_http_namespace" "internal" {
  name        = "internal"
  description = "Cloud Map namespace backing Service Connect for auth/orders/notifications"
}
