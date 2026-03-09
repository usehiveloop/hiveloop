# =============================================================================
# ElastiCache Module - Redis for LLMVault
# =============================================================================

locals {
  common_tags = {
    Module = "elasticache"
  }
}

# -----------------------------------------------------------------------------
# ElastiCache Subnet Group
# -----------------------------------------------------------------------------
resource "aws_elasticache_subnet_group" "main" {
  name        = "${var.name}-cache-subnet-group"
  description = "Subnet group for ${var.name} ElastiCache"
  subnet_ids  = var.subnet_ids

  tags = merge(local.common_tags, {
    Name = "${var.name}-cache-subnet-group"
  })
}

# -----------------------------------------------------------------------------
# Security Group for ElastiCache
# -----------------------------------------------------------------------------
resource "aws_security_group" "elasticache" {
  name_prefix = "${var.name}-cache-"
  description = "Security group for ${var.name} ElastiCache Redis"
  vpc_id      = var.vpc_id

  ingress {
    description     = "Redis from ECS tasks"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = var.allowed_security_group_ids
  }

  egress {
    description = "No outbound needed"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = []
  }

  tags = merge(local.common_tags, {
    Name = "${var.name}-cache"
  })

  lifecycle {
    create_before_destroy = true
  }
}

# -----------------------------------------------------------------------------
# ElastiCache Parameter Group
# Optimized for small instance with maxmemory
# -----------------------------------------------------------------------------
resource "aws_elasticache_parameter_group" "main" {
  family = "redis7"
  name   = "${var.name}-cache-params"

  description = "Custom parameters for ${var.name}"

  # Memory management - critical for small instance
  # Note: maxmemory cannot be set via parameter group, use node type sizing instead
  
  parameter {
    name  = "maxmemory-policy"
    value = "allkeys-lru"  # Evict least recently used when full
  }

  # Performance for small instance
  parameter {
    name  = "activedefrag"
    value = "yes"
  }

  # Slow log for debugging
  parameter {
    name  = "slowlog-log-slower-than"
    value = "10000"  # 10ms
  }

  parameter {
    name  = "slowlog-max-len"
    value = "128"
  }

  tags = local.common_tags
}

# -----------------------------------------------------------------------------
# ElastiCache Redis Cluster
# Single node (no cluster mode) for cost savings
# -----------------------------------------------------------------------------
resource "aws_elasticache_cluster" "main" {
  cluster_id = var.name

  engine               = "redis"
  engine_version       = var.engine_version
  node_type            = var.node_type
  num_cache_nodes      = 1  # Single node (no replication)
  parameter_group_name = aws_elasticache_parameter_group.main.name
  port                 = 6379

  # Network
  subnet_group_name  = aws_elasticache_subnet_group.main.name
  security_group_ids = [aws_security_group.elasticache.id]

  # Encryption disabled for small cache (adds overhead)
  # Note: transit_encryption_enabled requires auth_token which adds complexity
  # For this small instance, we rely on security groups for access control

  # Maintenance
  maintenance_window = var.maintenance_window
  snapshot_window    = var.snapshot_window

  # Snapshots (minimal retention for cost)
  snapshot_retention_limit = var.snapshot_retention_days

  # Auto minor version upgrades
  auto_minor_version_upgrade = true

  # Apply immediately for faster updates
  apply_immediately = true

  tags = merge(local.common_tags, {
    Name = var.name
  })
}
