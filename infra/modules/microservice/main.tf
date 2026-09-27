data "aws_region" "current" {}

locals {
  container_name = var.service_name
  collector_name = "${var.service_name}-otel-collector"

  otel_collector_config = yamlencode({
    receivers = {
      otlp = {
        protocols = {
          http = { endpoint = "0.0.0.0:4318" }
          grpc = { endpoint = "0.0.0.0:4317" }
        }
      }
    }
    processors = {
      batch = {}
    }
    exporters = {
      awsxray = {
        region = data.aws_region.current.region
      }
    }
    service = {
      pipelines = {
        traces = {
          receivers  = ["otlp"]
          processors = ["batch"]
          exporters  = ["awsxray"]
        }
      }
    }
  })
}

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.project_name}/${var.service_name}"
  retention_in_days = var.log_retention_days
}

resource "aws_ecs_task_definition" "this" {
  family                   = "${var.project_name}-${var.service_name}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.task_cpu
  memory                   = var.task_memory
  execution_role_arn       = var.execution_role_arn
  task_role_arn            = var.task_role_arn

  container_definitions = jsonencode([
    {
      name      = local.container_name
      image     = "${var.ecr_repository_url}:${var.image_tag}"
      essential = true

      portMappings = [
        { name = "app", containerPort = var.container_port, protocol = "tcp" }
      ]

      environment = concat(
        [
          { name = "PORT", value = tostring(var.container_port) },
          # The collector listens on localhost inside this same task --
          # Fargate/awsvpc tasks share one network namespace across containers.
          { name = "OTEL_EXPORTER_OTLP_ENDPOINT", value = "localhost:4318" },
        ],
        [for k, v in var.environment_variables : { name = k, value = v }]
      )

      secrets = [for k, arn in var.secrets : { name = k, valueFrom = arn }]

      dependsOn = [
        { containerName = local.collector_name, condition = "START" }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.this.name
          "awslogs-region"        = data.aws_region.current.region
          "awslogs-stream-prefix" = var.service_name
        }
      }
    },
    {
      name      = local.collector_name
      image     = var.otel_collector_image
      essential = false # a collector hiccup shouldn't take the app down with it

      environment = [
        { name = "AOT_CONFIG_CONTENT", value = local.otel_collector_config }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.this.name
          "awslogs-region"        = data.aws_region.current.region
          "awslogs-stream-prefix" = "${var.service_name}-otel"
        }
      }
    }
  ])
}



resource "aws_lb_target_group" "blue" {
  name        = "${var.project_name}-${var.service_name}-blue"
  port        = var.container_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = "/health"
    matcher             = "200"
    interval            = 15
    healthy_threshold   = 2
    unhealthy_threshold = 2
  }

  deregistration_delay = 30
}

resource "aws_lb_target_group" "green" {
  name        = "${var.project_name}-${var.service_name}-green"
  port        = var.container_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = "/health"
    matcher             = "200"
    interval            = 15
    healthy_threshold   = 2
    unhealthy_threshold = 2
  }

  deregistration_delay = 30
}

resource "aws_lb_listener_rule" "this" {
  listener_arn = var.alb_listener_arn
  priority     = var.listener_priority

  condition {
    path_pattern {
      values = [var.path_pattern]
    }
  }

  action {
    type = "forward"

    forward {
      target_group {
        arn    = aws_lb_target_group.blue.arn
        weight = 100
      }
      target_group {
        arn    = aws_lb_target_group.green.arn
        weight = 0
      }
    }
  }
}


resource "aws_ecs_service" "this" {
  name            = "${var.project_name}-${var.service_name}"
  cluster         = var.cluster_arn
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = 1

  availability_zone_rebalancing = "ENABLED"

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.ecs_tasks_sg_id]
    assign_public_ip = false
  }

  capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 100
    base              = 1
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.blue.arn
    container_name   = local.container_name
    container_port   = var.container_port

    advanced_configuration {
      alternate_target_group_arn = aws_lb_target_group.green.arn
      production_listener_rule   = aws_lb_listener_rule.this.arn
      role_arn                   = var.bluegreen_infra_role_arn
    }
  }

  service_connect_configuration {
    enabled   = true
    namespace = var.cloudmap_namespace_arn

    service {
      port_name      = "app"
      discovery_name = var.service_name

      client_alias {
        dns_name = "${var.service_name}.internal"
        port     = var.container_port
      }
    }

    log_configuration {
      log_driver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.this.name
        "awslogs-region"        = data.aws_region.current.region
        "awslogs-stream-prefix" = "${var.service_name}-service-connect"
      }
    }
  }

  deployment_controller {
    type = "ECS"
  }

  deployment_configuration {
    strategy              = "BLUE_GREEN"
    bake_time_in_minutes  = 2

  }

  health_check_grace_period_seconds = 30

  lifecycle {
    ignore_changes = [task_definition]
  }
}
