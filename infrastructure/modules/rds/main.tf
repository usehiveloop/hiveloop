# =============================================================================
# RDS Module - PostgreSQL for LLMVault
# =============================================================================

locals {
  db_name = var.db_name != "" ? var.db_name : replace(var.name, "-", "_")
  common_tags = {
    Module = "rds"
  }
}

# -----------------------------------------------------------------------------
# Database Subnet Group
# Places RDS in private subnets across multiple AZs
# -----------------------------------------------------------------------------
resource "aws_db_subnet_group" "main" {
  name        = "${var.name}-db-subnet-group"
  description = "Database subnet group for ${var.name}"
  subnet_ids  = var.subnet_ids

  tags = merge(local.common_tags, {
    Name = "${var.name}-db-subnet-group"
  })
}

# -----------------------------------------------------------------------------
# Security Group for RDS
# Allows PostgreSQL access only from ECS tasks
# -----------------------------------------------------------------------------
resource "aws_security_group" "rds" {
  name_prefix = "${var.name}-rds-"
  description = "Security group for ${var.name} RDS PostgreSQL"
  vpc_id      = var.vpc_id

  ingress {
    description     = "PostgreSQL from ECS tasks"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = var.allowed_security_group_ids
  }

  egress {
    description = "No outbound traffic needed"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = []
  }

  tags = merge(local.common_tags, {
    Name = "${var.name}-rds"
  })

  lifecycle {
    create_before_destroy = true
  }
}

# -----------------------------------------------------------------------------
# Generate Random Database Password
# -----------------------------------------------------------------------------
resource "random_password" "db_password" {
  length           = 32
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

# -----------------------------------------------------------------------------
# Store Credentials in Secrets Manager
# -----------------------------------------------------------------------------
resource "aws_secretsmanager_secret" "db_credentials" {
  name                    = "${var.name}/db-credentials"
  description             = "Database credentials for ${var.name}"
  recovery_window_in_days = 7  # Allow 7 days to recover if deleted

  tags = local.common_tags
}

resource "aws_secretsmanager_secret_version" "db_credentials" {
  secret_id = aws_secretsmanager_secret.db_credentials.id
  secret_string = jsonencode({
    username = var.db_username
    password = random_password.db_password.result
    host     = aws_db_instance.main.address
    port     = 5432
    dbname   = local.db_name
    # Full connection string for convenience
    database_url = "postgres://${var.db_username}:${random_password.db_password.result}@${aws_db_instance.main.address}:5432/${local.db_name}"
  })
}

# -----------------------------------------------------------------------------
# DB Parameter Group
# Custom PostgreSQL settings optimized for small instance
# -----------------------------------------------------------------------------
resource "aws_db_parameter_group" "main" {
  family = "postgres${split(".", var.engine_version)[0]}"
  name   = "${var.name}-pg"

  description = "Custom parameters for ${var.name}"

  # Optimize for small instance (1GB RAM)
  # Static parameters require pending-reboot apply method
  parameter {
    name         = "shared_buffers"
    value        = "262144"  # 256MB (25% of RAM)
    apply_method = "pending-reboot"
  }

  parameter {
    name         = "max_connections"
    value        = "100"
    apply_method = "pending-reboot"
  }

  # Dynamic parameters can use immediate apply
  parameter {
    name  = "effective_cache_size"
    value = "786432"  # 768MB (75% of RAM)
  }

  parameter {
    name  = "work_mem"
    value = "4096"  # 4MB
  }

  parameter {
    name         = "maintenance_work_mem"
    value        = "65536"  # 64MB
    apply_method = "pending-reboot"
  }

  parameter {
    name  = "log_min_duration_statement"
    value = "1000"  # Log slow queries (>1s)
  }

  tags = local.common_tags
}

# -----------------------------------------------------------------------------
# RDS Instance
# PostgreSQL database for LLMVault
# -----------------------------------------------------------------------------
resource "aws_db_instance" "main" {
  identifier = var.name

  # Engine
  engine         = "postgres"
  engine_version = var.engine_version
  instance_class = var.instance_class

  # Storage
  allocated_storage     = var.allocated_storage
  max_allocated_storage = var.max_allocated_storage  # Enable storage autoscaling
  storage_type          = "gp3"
  storage_encrypted     = true

  # Database
  db_name  = local.db_name
  username = var.db_username
  password = random_password.db_password.result

  # Network
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false

  # Parameter Group
  parameter_group_name = aws_db_parameter_group.main.name

  # Backup & Maintenance
  backup_retention_period = var.backup_retention_days
  backup_window           = var.backup_window           # UTC
  maintenance_window      = var.maintenance_window      # UTC

  # Deletion protection (disable for easier destroy in dev/staging)
  deletion_protection = var.deletion_protection
  skip_final_snapshot = var.skip_final_snapshot

  # Monitoring (minimal for cost savings)
  performance_insights_enabled = false
  monitoring_interval          = 0  # Disable enhanced monitoring

  # Auto minor version upgrades
  auto_minor_version_upgrade = true

  # Tags
  tags = merge(local.common_tags, {
    Name = var.name
  })
}
