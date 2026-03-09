# =============================================================================
# ECS Cluster Module
# Creates ECS cluster with CloudWatch logging
# =============================================================================

locals {
  common_tags = {
    Module = "ecs-cluster"
  }
}

# -----------------------------------------------------------------------------
# ECS Cluster
# -----------------------------------------------------------------------------
resource "aws_ecs_cluster" "main" {
  name = var.name

  setting {
    name  = "containerInsights"
    value = var.enable_container_insights ? "enabled" : "disabled"
  }

  tags = merge(local.common_tags, {
    Name = var.name
  })
}

# -----------------------------------------------------------------------------
# CloudWatch Log Group for ECS
# -----------------------------------------------------------------------------
resource "aws_cloudwatch_log_group" "ecs" {
  name              = "/ecs/${var.name}"
  retention_in_days = var.log_retention_days

  tags = local.common_tags
}

# -----------------------------------------------------------------------------
# CloudWatch Log Groups for Services (pre-created for organization)
# -----------------------------------------------------------------------------
resource "aws_cloudwatch_log_group" "services" {
  for_each = toset(var.services)

  name              = "/ecs/${var.name}/${each.value}"
  retention_in_days = var.log_retention_days

  tags = merge(local.common_tags, {
    Service = each.value
  })
}

# -----------------------------------------------------------------------------
# Capacity Providers
# FARGATE and FARGATE_SPOT for cost optimization
# -----------------------------------------------------------------------------
resource "aws_ecs_cluster_capacity_providers" "main" {
  cluster_name = aws_ecs_cluster.main.name

  capacity_providers = ["FARGATE", "FARGATE_SPOT"]

  default_capacity_provider_strategy {
    base              = 1
    weight            = 1
    capacity_provider = "FARGATE"
  }
}
