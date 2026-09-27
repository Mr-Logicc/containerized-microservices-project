module "auth" {
  source = "./modules/microservice"

  project_name      = var.project_name
  service_name      = "auth"
  container_port    = 8080
  path_pattern      = "/api/auth/*"
  listener_priority = 10

  cluster_arn         = aws_ecs_cluster.this.arn
  vpc_id              = module.vpc.vpc_id
  private_subnet_ids  = module.vpc.private_subnets
  ecs_tasks_sg_id     = aws_security_group.ecs_tasks.id
  alb_listener_arn    = aws_lb_listener.http.arn

  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  bluegreen_infra_role_arn = aws_iam_role.ecs_bluegreen_infra.arn

  cloudmap_namespace_arn = aws_service_discovery_http_namespace.internal.arn

  ecr_repository_url = aws_ecr_repository.services["auth"].repository_url
  image_tag          = var.container_image_tag

  environment_variables = {
    REDIS_ADDR = "${aws_elasticache_cluster.redis.cache_nodes[0].address}:6379"
  }

  secrets = {
    API_KEY = aws_secretsmanager_secret.api_key.arn
  }

  depends_on = [aws_ecs_cluster_capacity_providers.this]
}

module "orders" {
  source = "./modules/microservice"

  project_name      = var.project_name
  service_name      = "orders"
  container_port    = 8080
  path_pattern      = "/api/orders/*"
  listener_priority = 20

  cluster_arn         = aws_ecs_cluster.this.arn
  vpc_id              = module.vpc.vpc_id
  private_subnet_ids  = module.vpc.private_subnets
  ecs_tasks_sg_id     = aws_security_group.ecs_tasks.id
  alb_listener_arn    = aws_lb_listener.http.arn

  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  bluegreen_infra_role_arn = aws_iam_role.ecs_bluegreen_infra.arn

  cloudmap_namespace_arn = aws_service_discovery_http_namespace.internal.arn

  ecr_repository_url = aws_ecr_repository.services["orders"].repository_url
  image_tag          = var.container_image_tag

  environment_variables = {
    REDIS_ADDR        = "${aws_elasticache_cluster.redis.cache_nodes[0].address}:6379"
    NOTIFICATIONS_URL = "http://notifications.internal:8080"
  }

  secrets = {
    API_KEY = aws_secretsmanager_secret.api_key.arn
  }

  depends_on = [aws_ecs_cluster_capacity_providers.this]
}

module "notifications" {
  source = "./modules/microservice"

  project_name      = var.project_name
  service_name      = "notifications"
  container_port    = 8080
  path_pattern      = "/api/notifications/*"
  listener_priority = 30

  cluster_arn         = aws_ecs_cluster.this.arn
  vpc_id              = module.vpc.vpc_id
  private_subnet_ids  = module.vpc.private_subnets
  ecs_tasks_sg_id     = aws_security_group.ecs_tasks.id
  alb_listener_arn    = aws_lb_listener.http.arn

  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn
  bluegreen_infra_role_arn = aws_iam_role.ecs_bluegreen_infra.arn

  cloudmap_namespace_arn = aws_service_discovery_http_namespace.internal.arn

  ecr_repository_url = aws_ecr_repository.services["notifications"].repository_url
  image_tag          = var.container_image_tag

  depends_on = [aws_ecs_cluster_capacity_providers.this]
}
