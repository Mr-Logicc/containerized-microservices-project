resource "aws_security_group" "alb" {
  name        = "${var.project_name}-alb"
  description = "Allows inbound HTTP from the internet to the ALB."
  vpc_id      = module.vpc.vpc_id

  ingress {
    description = "HTTP from anywhere -- this is a demo; add ACM + an HTTPS listener before this goes anywhere real"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}


resource "aws_security_group" "ecs_tasks" {
  name        = "${var.project_name}-ecs-tasks"
  description = "Shared by auth, orders, and notifications tasks."
  vpc_id      = module.vpc.vpc_id

  egress {
    description = "Needed to reach ECR, Secrets Manager, and X-Ray via the NAT Gateway"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group_rule" "ecs_from_alb" {
  description              = "ALB - ECS tasks on the app port"
  type                     = "ingress"
  from_port                = 8080
  to_port                  = 8080
  protocol                 = "tcp"
  security_group_id        = aws_security_group.ecs_tasks.id
  source_security_group_id = aws_security_group.alb.id
}

resource "aws_security_group_rule" "ecs_from_ecs" {
  description       = "Lets orders call notifications (and any future service-to-service call) directly by Cloud Map DNS name"
  type              = "ingress"
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  security_group_id = aws_security_group.ecs_tasks.id
  self              = true
}
