import * as cdk from "aws-cdk-lib";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as ecs from "aws-cdk-lib/aws-ecs";
import * as elbv2 from "aws-cdk-lib/aws-elasticloadbalancingv2";
import * as logs from "aws-cdk-lib/aws-logs";
import * as secretsmanager from "aws-cdk-lib/aws-secretsmanager";
import * as kms from "aws-cdk-lib/aws-kms";
import * as iam from "aws-cdk-lib/aws-iam";
import { Construct } from "constructs";
import {
  EnvironmentConfig,
  envDomain,
  subdomain,
} from "../config/environments";
import { PROJECT_NAME, TAGS } from "../config/constants";

export interface ApiStackProps extends cdk.StackProps {
  config: EnvironmentConfig;
  vpc: ec2.IVpc;
  cluster: ecs.ICluster;
  ecsSecurityGroup: ec2.ISecurityGroup;
  apiTargetGroup: elbv2.IApplicationTargetGroup;
  dbSecret: cdk.aws_secretsmanager.ISecret;
  dbEndpoint: string;
  redisEndpoint: string;
  redisPort: string;
  zitadelDomain: string;
}

export class ApiStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: ApiStackProps) {
    super(scope, id, props);

    const { config, vpc, cluster, ecsSecurityGroup } = props;
    const prefix = `${PROJECT_NAME}-${config.name}`;

    // -------------------------------------------------------------------
    // KMS key for envelope encryption (replaces dev AEAD)
    // -------------------------------------------------------------------
    const kmsKey = new kms.Key(this, "KmsKey", {
      alias: `${prefix}/api-encryption`,
      description: `LLMVault ${config.name} API envelope encryption key`,
      enableKeyRotation: true,
    });

    // -------------------------------------------------------------------
    // Secrets
    // -------------------------------------------------------------------
    // Look up ZITADEL secrets by name (created in ZitadelStack, populated after init)
    const zitadelAdminPat = secretsmanager.Secret.fromSecretNameV2(
      this, "ZitadelAdminPat", `${prefix}/zitadel-admin-pat`
    );
    const zitadelApiClientId = secretsmanager.Secret.fromSecretNameV2(
      this, "ZitadelApiClientId", `${prefix}/zitadel-api-client-id`
    );
    const zitadelApiClientSecret = secretsmanager.Secret.fromSecretNameV2(
      this, "ZitadelApiClientSecret", `${prefix}/zitadel-api-client-secret`
    );
    const zitadelProjectId = secretsmanager.Secret.fromSecretNameV2(
      this, "ZitadelProjectId", `${prefix}/zitadel-project-id`
    );

    const jwtSigningKey = new secretsmanager.Secret(this, "JwtSigningKey", {
      secretName: `${prefix}/jwt-signing-key`,
      description: "JWT signing key for proxy tokens",
      generateSecretString: {
        passwordLength: 64,
        excludePunctuation: true,
      },
    });

    // -------------------------------------------------------------------
    // Task Definition
    // -------------------------------------------------------------------

    const taskDef = new ecs.FargateTaskDefinition(this, "TaskDef", {
      family: `${prefix}-api`,
      cpu: config.apiCpu,
      memoryLimitMiB: config.apiMemory,
      runtimePlatform: {
        cpuArchitecture: ecs.CpuArchitecture.ARM64,
        operatingSystemFamily: ecs.OperatingSystemFamily.LINUX,
      },
    });

    // KMS grant on task role (used at runtime by the app)
    kmsKey.grantEncryptDecrypt(taskDef.taskRole);

    // Grant execution role access to all project secrets.
    // fromSecretNameV2() partial ARNs don't include the random suffix,
    // so CDK's auto-grant doesn't match. Use a wildcard instead.
    taskDef.executionRole!.addToPrincipalPolicy(
      new iam.PolicyStatement({
        actions: ["secretsmanager:GetSecretValue"],
        resources: [
          `arn:aws:secretsmanager:${this.region}:${this.account}:secret:${prefix}/*`,
        ],
      })
    );

    const logGroup = new logs.LogGroup(this, "Logs", {
      logGroupName: `/ecs/${prefix}/api`,
      retention: logs.RetentionDays.ONE_WEEK,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    // -------------------------------------------------------------------
    // DATABASE_URL secret — constructed from RDS secret via dynamic refs
    // -------------------------------------------------------------------
    const dbUrlSecret = new secretsmanager.CfnSecret(this, "DatabaseUrl", {
      name: `${prefix}/database-url`,
      secretString: cdk.Fn.sub(
        "postgres://${username}:${password}@${host}:5432/llmvault?sslmode=require",
        {
          username: `{{resolve:secretsmanager:${props.dbSecret.secretName}:SecretString:username}}`,
          password: `{{resolve:secretsmanager:${props.dbSecret.secretName}:SecretString:password}}`,
          host: props.dbEndpoint,
        }
      ),
    });
    const dbUrlSecretRef = secretsmanager.Secret.fromSecretNameV2(
      this,
      "DatabaseUrlRef",
      `${prefix}/database-url`
    );
    dbUrlSecretRef.node.addDependency(dbUrlSecret);

    taskDef.addContainer("api", {
      containerName: "api",
      image: ecs.ContainerImage.fromRegistry("ghcr.io/llmvault/llmvault:dev"),
      portMappings: [{ containerPort: 8080, protocol: ecs.Protocol.TCP }],
      environment: {
        // Server
        PORT: "8080",
        ENVIRONMENT: config.name,
        LOG_LEVEL: "info",
        LOG_FORMAT: "json",

        // Redis
        REDIS_ADDR: `${props.redisEndpoint}:${props.redisPort}`,
        REDIS_DB: "0",
        REDIS_CACHE_TTL: "5m",

        // L1 Cache
        MEM_CACHE_TTL: "1m",
        MEM_CACHE_MAX_SIZE: "1000",

        // KMS — AWS KMS for envelope encryption
        KMS_TYPE: "awskms",
        KMS_KEY: kmsKey.keyArn,
        AWS_REGION: config.region,

        // ZITADEL
        ZITADEL_DOMAIN: props.zitadelDomain,

        // CORS
        CORS_ORIGINS: `https://${subdomain(config, "connect")},https://${envDomain(config)}`,
      },
      secrets: {
        DATABASE_URL: ecs.Secret.fromSecretsManager(dbUrlSecretRef),
        JWT_SIGNING_KEY: ecs.Secret.fromSecretsManager(jwtSigningKey),
        ZITADEL_ADMIN_PAT: ecs.Secret.fromSecretsManager(zitadelAdminPat),
        ZITADEL_CLIENT_ID: ecs.Secret.fromSecretsManager(zitadelApiClientId),
        ZITADEL_CLIENT_SECRET: ecs.Secret.fromSecretsManager(
          zitadelApiClientSecret
        ),
        ZITADEL_PROJECT_ID: ecs.Secret.fromSecretsManager(zitadelProjectId),
      },
      logging: ecs.LogDrivers.awsLogs({
        logGroup,
        streamPrefix: "api",
      }),
      // No container health check — distroless image has no shell.
      // ALB target group health check at /healthz handles liveness.
    });

    // -------------------------------------------------------------------
    // Fargate Service
    // -------------------------------------------------------------------

    const service = new ecs.FargateService(this, "Service", {
      serviceName: `${prefix}-api`,
      cluster,
      taskDefinition: taskDef,
      desiredCount: config.apiDesiredCount,
      securityGroups: [ecsSecurityGroup],
      vpcSubnets: { subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS },
      assignPublicIp: false,
      circuitBreaker: { enable: true, rollback: true },
      enableExecuteCommand: true,
    });

    service.attachToApplicationTargetGroup(
      props.apiTargetGroup as elbv2.ApplicationTargetGroup
    );

    // -------------------------------------------------------------------
    // Outputs
    // -------------------------------------------------------------------
    new cdk.CfnOutput(this, "KmsKeyArn", {
      value: kmsKey.keyArn,
      description: "KMS key ARN for envelope encryption",
    });

    // -------------------------------------------------------------------
    // Tags
    // -------------------------------------------------------------------
    cdk.Tags.of(this).add("Environment", config.name);
    Object.entries(TAGS).forEach(([k, v]) => cdk.Tags.of(this).add(k, v));
  }
}
