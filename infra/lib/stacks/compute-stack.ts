import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as acm from 'aws-cdk-lib/aws-certificatemanager';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as ecs from 'aws-cdk-lib/aws-ecs';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as kms from 'aws-cdk-lib/aws-kms';
import * as ssm from 'aws-cdk-lib/aws-ssm';
import { EnvironmentConfig, ssmPath, SSM_PARAMS } from '../config';
import { SecurityGroups } from './network-stack';
import { DataStack } from './data-stack';
import { SharedStack } from './shared-stack';
import { FargateService } from '../constructs/fargate-service';

export interface ComputeStackProps extends cdk.StackProps {
  readonly config: EnvironmentConfig;
  readonly vpc: ec2.IVpc;
  readonly securityGroups: SecurityGroups;
  readonly kmsKey: kms.IKey;
  readonly taskRole: iam.IRole;
  readonly executionRole: iam.IRole;
  readonly instanceProfile: iam.IInstanceProfile;
  readonly data?: DataStack;
  readonly shared: SharedStack;
}

/**
 * Application compute layer.
 *
 *   phase 'fargate' → ALB + 3 ECS Fargate services
 *   phase 'ec2'     → single EC2 instance running Docker Compose + Caddy
 *
 * DNS is managed in Cloudflare. This stack outputs the ALB DNS name (or
 * Elastic IP) so you can create CNAME/A records in the Cloudflare dashboard.
 *
 * ACM certificates are validated via DNS — add the CNAME records shown in
 * the AWS ACM console to Cloudflare during the first deploy.
 */
export class ComputeStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: ComputeStackProps) {
    super(scope, id, props);

    if (props.config.phase === 'fargate') {
      this.deployFargate(props);
    } else {
      this.deployEc2(props);
    }
  }

  // =========================================================================
  // Fargate mode
  // =========================================================================

  private deployFargate(props: ComputeStackProps) {
    const { config } = props;

    // --- ACM certificate (manual DNS validation via Cloudflare) ---
    //
    // On first deploy CloudFormation will pause here waiting for validation.
    // Open the AWS ACM console → copy the CNAME records → add them in
    // Cloudflare. Once validated the deploy continues automatically.

    const cert = new acm.Certificate(this, 'Cert', {
      domainName: config.domains.root,
      subjectAlternativeNames: [
        config.domains.api,
        config.domains.auth,
      ],
      validation: acm.CertificateValidation.fromDns(),
    });

    // --- ECS Cluster ---

    const cluster = new ecs.Cluster(this, 'Cluster', {
      clusterName: `llmvault-${config.envName}`,
      vpc: props.vpc,
      containerInsights: true,
    });

    // --- ALB (single ALB, host-based routing) ---

    const alb = new elbv2.ApplicationLoadBalancer(this, 'Alb', {
      vpc: props.vpc,
      internetFacing: true,
      securityGroup: props.securityGroups.alb,
      vpcSubnets: { subnetType: ec2.SubnetType.PUBLIC },
    });

    alb.addRedirect({ sourcePort: 80, targetPort: 443 });

    const httpsListener = alb.addListener('Https', {
      port: 443,
      certificates: [cert],
      defaultAction: elbv2.ListenerAction.fixedResponse(404, {
        contentType: 'text/plain',
        messageBody: 'Not Found',
      }),
    });

    // --- Helper: SSM parameter reference ---

    const ssmSecret = (name: string): ecs.Secret =>
      ecs.Secret.fromSsmParameter(
        ssm.StringParameter.fromSecureStringParameterAttributes(this, `Ssm-${name}`, {
          parameterName: ssmPath(config.envName, name),
        }),
      );

    const smSecret = (field: string): ecs.Secret =>
      ecs.Secret.fromSecretsManager(props.data!.rds.secret, field);

    // --- Shared Fargate props ---

    const subnets: ec2.SubnetSelection = { subnetType: ec2.SubnetType.PUBLIC };

    // --- API service ---

    const api = new FargateService(this, 'Api', {
      cluster,
      serviceName: 'api',
      image: ecs.ContainerImage.fromEcrRepository(props.shared.apiRepo, 'latest'),
      containerPort: 8080,
      cpu: config.fargate.api.cpu,
      memoryLimitMiB: config.fargate.api.memory,
      desiredCount: config.tasksPerService,
      healthCheckPath: '/healthz',
      taskRole: props.taskRole,
      executionRole: props.executionRole,
      securityGroups: [props.securityGroups.api],
      subnets,
      environment: {
        ENVIRONMENT: config.envName === 'prod' ? 'production' : 'development',
        PORT: '8080',
        LOG_LEVEL: 'info',
        LOG_FORMAT: 'json',
        KMS_TYPE: 'awskms',
        KMS_KEY: props.kmsKey.keyArn,
        AWS_REGION: config.region,
        REDIS_ADDR: props.data
          ? `${props.data.redis.endpoint}:${props.data.redis.port}`
          : 'localhost:6379',
        REDIS_DB: '0',
        REDIS_CACHE_TTL: '30m',
        MEM_CACHE_TTL: '5m',
        MEM_CACHE_MAX_SIZE: '10000',
        CORS_ORIGINS: `https://${config.domains.root},https://${config.domains.connect}`,
        ZITADEL_DOMAIN: `https://${config.domains.auth}`,
      },
      secrets: {
        DATABASE_URL: ssmSecret(SSM_PARAMS.databaseUrl),
        REDIS_PASSWORD: ssmSecret(SSM_PARAMS.redisPassword),
        JWT_SIGNING_KEY: ssmSecret(SSM_PARAMS.jwtSigningKey),
        ZITADEL_CLIENT_ID: ssmSecret(SSM_PARAMS.zitadelClientId),
        ZITADEL_CLIENT_SECRET: ssmSecret(SSM_PARAMS.zitadelClientSecret),
        ZITADEL_ADMIN_PAT: ssmSecret(SSM_PARAMS.zitadelAdminPat),
        ZITADEL_PROJECT_ID: ssmSecret(SSM_PARAMS.zitadelProjectId),
      },
    });

    // --- Web service ---

    const web = new FargateService(this, 'Web', {
      cluster,
      serviceName: 'web',
      image: ecs.ContainerImage.fromEcrRepository(props.shared.webRepo, 'latest'),
      containerPort: 3000,
      cpu: config.fargate.web.cpu,
      memoryLimitMiB: config.fargate.web.memory,
      desiredCount: config.tasksPerService,
      healthCheckPath: '/',
      taskRole: props.taskRole,
      executionRole: props.executionRole,
      securityGroups: [props.securityGroups.web],
      subnets,
      environment: {
        NODE_ENV: 'production',
        NEXT_PUBLIC_API_URL: `https://${config.domains.api}`,
        NEXT_PUBLIC_ZITADEL_ISSUER: `https://${config.domains.auth}`,
        NEXT_PUBLIC_CONNECT_URL: `https://${config.domains.connect}`,
      },
      secrets: {
        NEXT_PUBLIC_ZITADEL_CLIENT_ID: ssmSecret(SSM_PARAMS.zitadelDashboardClientId),
      },
    });

    // --- ZITADEL service ---

    const zitadel = new FargateService(this, 'Zitadel', {
      cluster,
      serviceName: 'zitadel',
      image: ecs.ContainerImage.fromRegistry(config.zitadelImage),
      containerPort: 8080,
      cpu: config.fargate.zitadel.cpu,
      memoryLimitMiB: config.fargate.zitadel.memory,
      desiredCount: config.tasksPerService,
      healthCheckPath: '/debug/ready',
      taskRole: props.taskRole,
      executionRole: props.executionRole,
      securityGroups: [props.securityGroups.zitadel],
      subnets,
      command: ['start-from-init'],
      environment: {
        ZITADEL_PORT: '8080',
        ZITADEL_EXTERNALDOMAIN: config.domains.auth,
        ZITADEL_EXTERNALPORT: '443',
        ZITADEL_EXTERNALSECURE: 'true',
        ZITADEL_TLS_ENABLED: 'false',
        ZITADEL_DATABASE_POSTGRES_HOST: props.data!.rds.instance.dbInstanceEndpointAddress,
        ZITADEL_DATABASE_POSTGRES_PORT: '5432',
        ZITADEL_DATABASE_POSTGRES_DATABASE: 'zitadel',
        ZITADEL_DATABASE_POSTGRES_USER_USERNAME: 'zitadel',
        ZITADEL_DATABASE_POSTGRES_ADMIN_SSL_MODE: 'require',
        ZITADEL_DATABASE_POSTGRES_USER_SSL_MODE: 'require',
        ZITADEL_FIRSTINSTANCE_ORG_HUMAN_PASSWORDCHANGEREQUIRED: 'false',
        ZITADEL_FIRSTINSTANCE_ORG_MACHINE_MACHINE_USERNAME: 'llmvault-admin',
        ZITADEL_FIRSTINSTANCE_ORG_MACHINE_MACHINE_NAME: 'LLMVault Admin SA',
        ZITADEL_FIRSTINSTANCE_ORG_MACHINE_PAT_EXPIRATIONDATE: '2099-01-01T00:00:00Z',
        ZITADEL_FIRSTINSTANCE_PATPATH: '/tmp/admin.pat',
        ZITADEL_DEFAULTINSTANCE_FEATURES_LOGINV2_REQUIRED: 'false',
      },
      secrets: {
        ZITADEL_MASTERKEY: ssmSecret(SSM_PARAMS.zitadelMasterkey),
        ZITADEL_DATABASE_POSTGRES_ADMIN_USERNAME: smSecret('username'),
        ZITADEL_DATABASE_POSTGRES_ADMIN_PASSWORD: smSecret('password'),
        ZITADEL_DATABASE_POSTGRES_USER_PASSWORD: ssmSecret(SSM_PARAMS.zitadelDbPassword),
      },
    });

    // --- ALB listener rules (host-based routing) ---

    httpsListener.addTargetGroups('ApiRule', {
      priority: 10,
      conditions: [elbv2.ListenerCondition.hostHeaders([config.domains.api])],
      targetGroups: [api.targetGroup],
    });

    httpsListener.addTargetGroups('ZitadelRule', {
      priority: 20,
      conditions: [elbv2.ListenerCondition.hostHeaders([config.domains.auth])],
      targetGroups: [zitadel.targetGroup],
    });

    httpsListener.addTargetGroups('WebRule', {
      priority: 30,
      conditions: [elbv2.ListenerCondition.hostHeaders([
        config.domains.root,
        `www.${config.domains.root}`,
      ])],
      targetGroups: [web.targetGroup],
    });

    // --- Outputs: create these CNAME records in Cloudflare ---

    new cdk.CfnOutput(this, 'AlbDnsName', {
      value: alb.loadBalancerDnsName,
      description: 'ALB DNS name — create CNAME records in Cloudflare pointing to this',
    });

    new cdk.CfnOutput(this, 'CloudflareDnsRecords', {
      value: [
        `${config.domains.root}  → CNAME → ${alb.loadBalancerDnsName}`,
        `${config.domains.api}   → CNAME → ${alb.loadBalancerDnsName}`,
        `${config.domains.auth}  → CNAME → ${alb.loadBalancerDnsName}`,
      ].join(' | '),
      description: 'DNS records to add in Cloudflare (all point to the ALB)',
    });
  }

  // =========================================================================
  // EC2 mode — single Docker host with Caddy TLS
  // =========================================================================

  private deployEc2(props: ComputeStackProps) {
    const { config } = props;

    const eip = new ec2.CfnEIP(this, 'Eip', {
      tags: [{ key: 'Name', value: `llmvault-${config.envName}` }],
    });

    const userData = ec2.UserData.forLinux();
    userData.addCommands(
      ...this.buildEc2UserData(config, props.shared, this.account, config.region),
    );

    const instance = new ec2.Instance(this, 'Instance', {
      instanceType: new ec2.InstanceType(config.ec2InstanceType),
      machineImage: ec2.MachineImage.latestAmazonLinux2023({ cpuType: ec2.AmazonLinuxCpuType.ARM_64 }),
      vpc: props.vpc,
      vpcSubnets: { subnetType: ec2.SubnetType.PUBLIC },
      securityGroup: props.securityGroups.ec2,
      role: (props.instanceProfile as iam.InstanceProfile).role!,
      userData,
      blockDevices: [{
        deviceName: '/dev/xvda',
        volume: ec2.BlockDeviceVolume.ebs(30, {
          volumeType: ec2.EbsDeviceVolumeType.GP3,
          encrypted: true,
        }),
      }],
      requireImdsv2: true,
    });

    new ec2.CfnEIPAssociation(this, 'EipAssoc', {
      allocationId: eip.attrAllocationId,
      instanceId: instance.instanceId,
    });

    // --- Outputs: create these A records in Cloudflare ---

    new cdk.CfnOutput(this, 'ElasticIp', {
      value: eip.attrPublicIp,
      description: 'Elastic IP — create A records in Cloudflare pointing to this',
    });

    new cdk.CfnOutput(this, 'CloudflareDnsRecords', {
      value: [
        `${config.domains.root}  → A → ${eip.attrPublicIp}`,
        `${config.domains.api}   → A → ${eip.attrPublicIp}`,
        `${config.domains.auth}  → A → ${eip.attrPublicIp}`,
      ].join(' | '),
      description: 'DNS records to add in Cloudflare (all point to the Elastic IP)',
    });
  }

  private buildEc2UserData(
    config: EnvironmentConfig,
    shared: SharedStack,
    accountId: string,
    region: string,
  ): string[] {
    const ecrBase = `${accountId}.dkr.ecr.${region}.amazonaws.com`;
    const env = config.envName;

    return [
      '#!/bin/bash',
      'set -euo pipefail',
      '',
      '# Install Docker + Compose',
      'dnf install -y docker',
      'systemctl enable --now docker',
      'usermod -aG docker ec2-user',
      '',
      'COMPOSE_VERSION="v2.32.4"',
      'ARCH=$(uname -m)',
      'curl -fsSL "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-${ARCH}" -o /usr/local/bin/docker-compose',
      'chmod +x /usr/local/bin/docker-compose',
      '',
      '# Install Caddy',
      'dnf install -y yum-utils',
      'dnf copr enable -y @caddy/caddy epel-9-aarch64',
      'dnf install -y caddy',
      '',
      '# ECR login',
      `aws ecr get-login-password --region ${region} | docker login --username AWS --password-stdin ${ecrBase}`,
      '',
      '# Create working directory',
      'mkdir -p /opt/llmvault',
      'cd /opt/llmvault',
      '',
      '# Fetch secrets from SSM',
      `get_ssm() { aws ssm get-parameter --region ${region} --name "$1" --with-decryption --query 'Parameter.Value' --output text 2>/dev/null || echo ""; }`,
      '',
      `DB_PASSWORD=$(get_ssm "/llmvault/${env}/database-url" || echo "placeholder")`,
      `REDIS_PASSWORD=$(get_ssm "${ssmPath(env, SSM_PARAMS.redisPassword)}")`,
      `JWT_SIGNING_KEY=$(get_ssm "${ssmPath(env, SSM_PARAMS.jwtSigningKey)}")`,
      `ZITADEL_MASTERKEY=$(get_ssm "${ssmPath(env, SSM_PARAMS.zitadelMasterkey)}")`,
      `ZITADEL_DB_PASSWORD=$(get_ssm "${ssmPath(env, SSM_PARAMS.zitadelDbPassword)}")`,
      `ZITADEL_CLIENT_ID=$(get_ssm "${ssmPath(env, SSM_PARAMS.zitadelClientId)}")`,
      `ZITADEL_CLIENT_SECRET=$(get_ssm "${ssmPath(env, SSM_PARAMS.zitadelClientSecret)}")`,
      `ZITADEL_ADMIN_PAT=$(get_ssm "${ssmPath(env, SSM_PARAMS.zitadelAdminPat)}")`,
      `ZITADEL_PROJECT_ID=$(get_ssm "${ssmPath(env, SSM_PARAMS.zitadelProjectId)}")`,
      '',
      '# Generate docker-compose.yml',
      `cat > /opt/llmvault/docker-compose.yml <<'COMPOSE'`,
      'services:',
      '  postgres:',
      '    image: postgres:17-alpine',
      '    environment:',
      '      POSTGRES_DB: llmvault',
      '      POSTGRES_USER: llmvault',
      '      POSTGRES_PASSWORD: llmvault_local',
      '    volumes:',
      '      - pgdata:/var/lib/postgresql/data',
      '    healthcheck:',
      '      test: ["CMD-SHELL", "pg_isready -U llmvault"]',
      '      interval: 2s',
      '      timeout: 5s',
      '      retries: 15',
      '',
      '  redis:',
      '    image: redis:7-alpine',
      '    command: redis-server --appendonly yes --maxmemory 256mb --maxmemory-policy allkeys-lru',
      '    healthcheck:',
      '      test: ["CMD", "redis-cli", "ping"]',
      '      interval: 2s',
      '      timeout: 5s',
      '      retries: 5',
      '',
      '  zitadel:',
      `    image: ${config.zitadelImage}`,
      '    user: "0"',
      '    command: start-from-init --masterkey "${ZITADEL_MASTERKEY}"',
      '    environment:',
      '      ZITADEL_PORT: 8080',
      `      ZITADEL_EXTERNALDOMAIN: ${config.domains.auth}`,
      '      ZITADEL_EXTERNALPORT: 443',
      '      ZITADEL_EXTERNALSECURE: "true"',
      '      ZITADEL_TLS_ENABLED: "false"',
      '      ZITADEL_DATABASE_POSTGRES_HOST: postgres',
      '      ZITADEL_DATABASE_POSTGRES_PORT: 5432',
      '      ZITADEL_DATABASE_POSTGRES_DATABASE: zitadel',
      '      ZITADEL_DATABASE_POSTGRES_ADMIN_USERNAME: llmvault',
      '      ZITADEL_DATABASE_POSTGRES_ADMIN_PASSWORD: llmvault_local',
      '      ZITADEL_DATABASE_POSTGRES_USER_USERNAME: zitadel',
      '      ZITADEL_DATABASE_POSTGRES_USER_PASSWORD: "${ZITADEL_DB_PASSWORD}"',
      '      ZITADEL_DATABASE_POSTGRES_ADMIN_SSL_MODE: disable',
      '      ZITADEL_DATABASE_POSTGRES_USER_SSL_MODE: disable',
      '      ZITADEL_FIRSTINSTANCE_ORG_HUMAN_PASSWORDCHANGEREQUIRED: "false"',
      '      ZITADEL_FIRSTINSTANCE_ORG_MACHINE_MACHINE_USERNAME: llmvault-admin',
      '      ZITADEL_FIRSTINSTANCE_ORG_MACHINE_MACHINE_NAME: LLMVault Admin SA',
      '      ZITADEL_FIRSTINSTANCE_ORG_MACHINE_PAT_EXPIRATIONDATE: "2099-01-01T00:00:00Z"',
      '      ZITADEL_FIRSTINSTANCE_PATPATH: /zitadel/bootstrap/admin.pat',
      '      ZITADEL_DEFAULTINSTANCE_FEATURES_LOGINV2_REQUIRED: "false"',
      '    volumes:',
      '      - zitadel_bootstrap:/zitadel/bootstrap:rw',
      '    healthcheck:',
      '      test: ["CMD", "/app/zitadel", "ready"]',
      '      interval: 5s',
      '      timeout: 30s',
      '      retries: 15',
      '      start_period: 30s',
      '    depends_on:',
      '      postgres:',
      '        condition: service_healthy',
      '',
      '  api:',
      `    image: ${ecrBase}/llmvault/api:latest`,
      '    environment:',
      `      ENVIRONMENT: development`,
      '      PORT: "8080"',
      '      LOG_LEVEL: info',
      '      LOG_FORMAT: json',
      '      DATABASE_URL: postgres://llmvault:llmvault_local@postgres:5432/llmvault?sslmode=disable',
      '      KMS_TYPE: aead',
      '      KMS_KEY: "${KMS_KEY}"',
      '      REDIS_ADDR: redis:6379',
      '      REDIS_DB: "0"',
      '      REDIS_CACHE_TTL: 30m',
      '      MEM_CACHE_TTL: 5m',
      '      MEM_CACHE_MAX_SIZE: "10000"',
      '      JWT_SIGNING_KEY: "${JWT_SIGNING_KEY}"',
      `      CORS_ORIGINS: "https://${config.domains.root},https://${config.domains.connect}"`,
      `      ZITADEL_DOMAIN: "https://${config.domains.auth}"`,
      '      ZITADEL_CLIENT_ID: "${ZITADEL_CLIENT_ID}"',
      '      ZITADEL_CLIENT_SECRET: "${ZITADEL_CLIENT_SECRET}"',
      '      ZITADEL_ADMIN_PAT: "${ZITADEL_ADMIN_PAT}"',
      '      ZITADEL_PROJECT_ID: "${ZITADEL_PROJECT_ID}"',
      '    depends_on:',
      '      postgres:',
      '        condition: service_healthy',
      '      redis:',
      '        condition: service_healthy',
      '',
      '  web:',
      `    image: ${ecrBase}/llmvault/web:latest`,
      '    environment:',
      '      NODE_ENV: production',
      `      NEXT_PUBLIC_API_URL: "https://${config.domains.api}"`,
      `      NEXT_PUBLIC_ZITADEL_ISSUER: "https://${config.domains.auth}"`,
      `      NEXT_PUBLIC_CONNECT_URL: "https://${config.domains.connect}"`,
      '',
      'volumes:',
      '  pgdata:',
      '  zitadel_bootstrap:',
      'COMPOSE',
      '',
      '# Generate .env for docker-compose variable substitution',
      'cat > /opt/llmvault/.env <<DOTENV',
      'ZITADEL_MASTERKEY=${ZITADEL_MASTERKEY}',
      'ZITADEL_DB_PASSWORD=${ZITADEL_DB_PASSWORD}',
      'JWT_SIGNING_KEY=${JWT_SIGNING_KEY}',
      'KMS_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=',
      'ZITADEL_CLIENT_ID=${ZITADEL_CLIENT_ID}',
      'ZITADEL_CLIENT_SECRET=${ZITADEL_CLIENT_SECRET}',
      'ZITADEL_ADMIN_PAT=${ZITADEL_ADMIN_PAT}',
      'ZITADEL_PROJECT_ID=${ZITADEL_PROJECT_ID}',
      'DOTENV',
      '',
      '# Generate Caddyfile',
      `cat > /etc/caddy/Caddyfile <<'CADDY'`,
      `${config.domains.api} {`,
      '  reverse_proxy localhost:8080',
      '}',
      '',
      `${config.domains.root} {`,
      '  reverse_proxy localhost:3000',
      '}',
      '',
      `${config.domains.auth} {`,
      '  reverse_proxy localhost:8085',
      '}',
      'CADDY',
      '',
      '# Map container ports to host',
      'cd /opt/llmvault',
      'docker-compose up -d',
      '',
      '# Start Caddy (auto TLS via Let\'s Encrypt)',
      'systemctl enable --now caddy',
      '',
      '# Create deploy script for CI/CD (SSM RunCommand)',
      'cat > /opt/llmvault/deploy.sh <<\'DEPLOY\'',
      '#!/bin/bash',
      'set -euo pipefail',
      'cd /opt/llmvault',
      `aws ecr get-login-password --region ${region} | docker login --username AWS --password-stdin ${ecrBase}`,
      'docker-compose pull',
      'docker-compose up -d --remove-orphans',
      'docker image prune -f',
      'DEPLOY',
      'chmod +x /opt/llmvault/deploy.sh',
    ];
  }
}
