import * as cdk from "aws-cdk-lib";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as ecs from "aws-cdk-lib/aws-ecs";
import * as elbv2 from "aws-cdk-lib/aws-elasticloadbalancingv2";
import * as logs from "aws-cdk-lib/aws-logs";
import * as secretsmanager from "aws-cdk-lib/aws-secretsmanager";
import { Construct } from "constructs";
import {
  EnvironmentConfig,
  subdomain,
} from "../config/environments";
import { PROJECT_NAME, TAGS } from "../config/constants";

export interface ZitadelStackProps extends cdk.StackProps {
  config: EnvironmentConfig;
  vpc: ec2.IVpc;
  cluster: ecs.ICluster;
  ecsSecurityGroup: ec2.ISecurityGroup;
  zitadelTargetGroup: elbv2.IApplicationTargetGroup;
  zitadelLoginTargetGroup: elbv2.IApplicationTargetGroup;
  dbSecret: cdk.aws_secretsmanager.ISecret;
  dbEndpoint: string;
}

export class ZitadelStack extends cdk.Stack {
  public readonly masterKeySecret: secretsmanager.ISecret;
  public readonly zitadelDbPasswordSecret: secretsmanager.ISecret;
  public readonly adminPatSecret: secretsmanager.ISecret;
  public readonly projectIdSecret: secretsmanager.ISecret;
  public readonly apiClientIdSecret: secretsmanager.ISecret;
  public readonly apiClientSecretSecret: secretsmanager.ISecret;

  constructor(scope: Construct, id: string, props: ZitadelStackProps) {
    super(scope, id, props);

    const { config, vpc, cluster, ecsSecurityGroup } = props;
    const prefix = `${PROJECT_NAME}-${config.name}`;
    const authDomain = subdomain(config, "auth");

    // -------------------------------------------------------------------
    // Secrets
    // -------------------------------------------------------------------

    this.masterKeySecret = new secretsmanager.Secret(this, "MasterKey", {
      secretName: `${prefix}/zitadel-masterkey`,
      description: "ZITADEL master key",
      generateSecretString: {
        passwordLength: 32,
        excludePunctuation: true,
      },
    });

    this.zitadelDbPasswordSecret = new secretsmanager.Secret(
      this,
      "ZitadelDbPassword",
      {
        secretName: `${prefix}/zitadel-db-password`,
        description: "ZITADEL database user password",
        generateSecretString: {
          passwordLength: 32,
          excludePunctuation: true,
        },
      }
    );

    // Admin human user password (for ZITADEL console login)
    const adminPasswordSecret = new secretsmanager.Secret(
      this,
      "AdminPassword",
      {
        secretName: `${prefix}/zitadel-admin-password`,
        description: "ZITADEL admin human user password",
        generateSecretString: {
          passwordLength: 24,
          excludePunctuation: false,
        },
      }
    );

    // Admin machine user PAT — populate after init
    this.adminPatSecret = new secretsmanager.Secret(this, "AdminPat", {
      secretName: `${prefix}/zitadel-admin-pat`,
      description:
        "ZITADEL admin machine user PAT — populate manually after init",
    });

    // init.sh outputs — populate after running bootstrap script
    this.projectIdSecret = new secretsmanager.Secret(this, "ProjectId", {
      secretName: `${prefix}/zitadel-project-id`,
      description: "ZITADEL project ID — populate after init",
    });

    this.apiClientIdSecret = new secretsmanager.Secret(
      this,
      "ApiClientId",
      {
        secretName: `${prefix}/zitadel-api-client-id`,
        description: "ZITADEL API app client ID — populate after init",
      }
    );

    this.apiClientSecretSecret = new secretsmanager.Secret(
      this,
      "ApiClientSecret",
      {
        secretName: `${prefix}/zitadel-api-client-secret`,
        description: "ZITADEL API app client secret — populate after init",
      }
    );

    const dashboardClientIdSecret = new secretsmanager.Secret(
      this,
      "DashboardClientId",
      {
        secretName: `${prefix}/zitadel-dashboard-client-id`,
        description:
          "ZITADEL OIDC dashboard client ID — populate after init",
      }
    );

    // -------------------------------------------------------------------
    // ZITADEL API Service
    // -------------------------------------------------------------------

    const zitadelTaskDef = new ecs.FargateTaskDefinition(
      this,
      "ZitadelTaskDef",
      {
        family: `${prefix}-zitadel`,
        cpu: config.zitadelCpu,
        memoryLimitMiB: config.zitadelMemory,
        runtimePlatform: {
          cpuArchitecture: ecs.CpuArchitecture.ARM64,
          operatingSystemFamily: ecs.OperatingSystemFamily.LINUX,
        },
      }
    );

    const zitadelLogGroup = new logs.LogGroup(this, "ZitadelLogs", {
      logGroupName: `/ecs/${prefix}/zitadel`,
      retention: logs.RetentionDays.ONE_WEEK,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    zitadelTaskDef.addContainer("zitadel", {
      containerName: "zitadel",
      image: ecs.ContainerImage.fromRegistry(
        "ghcr.io/zitadel/zitadel:v4.12.1"
      ),
      command: ["start", "--masterkeyFromEnv"],
      portMappings: [{ containerPort: 8080, protocol: ecs.Protocol.TCP }],
      environment: {
        ZITADEL_EXTERNALDOMAIN: authDomain,
        ZITADEL_EXTERNALPORT: "443",
        ZITADEL_EXTERNALSECURE: "true",
        ZITADEL_TLS_ENABLED: "false",

        ZITADEL_DATABASE_POSTGRES_HOST: props.dbEndpoint,
        ZITADEL_DATABASE_POSTGRES_PORT: "5432",
        ZITADEL_DATABASE_POSTGRES_DATABASE: "zitadel",
        ZITADEL_DATABASE_POSTGRES_USER_USERNAME: "zitadel",
        ZITADEL_DATABASE_POSTGRES_ADMIN_USERNAME: "llmvault",
        ZITADEL_DATABASE_POSTGRES_ADMIN_SSL_MODE: "require",
        ZITADEL_DATABASE_POSTGRES_USER_SSL_MODE: "require",
      },
      secrets: {
        ZITADEL_MASTERKEY: ecs.Secret.fromSecretsManager(
          this.masterKeySecret
        ),
        ZITADEL_DATABASE_POSTGRES_USER_PASSWORD:
          ecs.Secret.fromSecretsManager(this.zitadelDbPasswordSecret),
        ZITADEL_DATABASE_POSTGRES_ADMIN_PASSWORD:
          ecs.Secret.fromSecretsManager(props.dbSecret, "password"),
      },
      logging: ecs.LogDrivers.awsLogs({
        logGroup: zitadelLogGroup,
        streamPrefix: "zitadel",
      }),
      healthCheck: {
        command: ["CMD", "/app/zitadel", "ready"],
        interval: cdk.Duration.seconds(30),
        timeout: cdk.Duration.seconds(5),
        retries: 3,
        startPeriod: cdk.Duration.seconds(120),
      },
    });

    const zitadelService = new ecs.FargateService(this, "ZitadelService", {
      serviceName: `${prefix}-zitadel`,
      cluster,
      taskDefinition: zitadelTaskDef,
      desiredCount: config.zitadelDesiredCount,
      securityGroups: [ecsSecurityGroup],
      vpcSubnets: { subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS },
      assignPublicIp: false,
      circuitBreaker: { enable: true, rollback: true },
      enableExecuteCommand: true,
    });

    zitadelService.attachToApplicationTargetGroup(
      props.zitadelTargetGroup as elbv2.ApplicationTargetGroup
    );

    // -------------------------------------------------------------------
    // ZITADEL Login v2 Service (Next.js UI)
    // ALB routes auth.llmvault.dev/ui/v2/login/* here
    // -------------------------------------------------------------------

    const loginTaskDef = new ecs.FargateTaskDefinition(
      this,
      "LoginTaskDef",
      {
        family: `${prefix}-zitadel-login`,
        cpu: 256,
        memoryLimitMiB: 512,
        runtimePlatform: {
          cpuArchitecture: ecs.CpuArchitecture.ARM64,
          operatingSystemFamily: ecs.OperatingSystemFamily.LINUX,
        },
      }
    );

    const loginLogGroup = new logs.LogGroup(this, "LoginLogs", {
      logGroupName: `/ecs/${prefix}/zitadel-login`,
      retention: logs.RetentionDays.ONE_WEEK,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    loginTaskDef.addContainer("zitadel-login", {
      containerName: "zitadel-login",
      image: ecs.ContainerImage.fromRegistry(
        "ghcr.io/zitadel/zitadel-login:v4.12.1"
      ),
      portMappings: [{ containerPort: 3000, protocol: ecs.Protocol.TCP }],
      environment: {
        // Login v2 talks to ZITADEL API internally via service discovery
        // Use the ALB since both are behind it
        ZITADEL_API_URL: `https://${authDomain}`,
        NEXT_PUBLIC_BASE_PATH: "/ui/v2/login",
        CUSTOM_REQUEST_HEADERS: `Host:${authDomain}`,
      },
      secrets: {
        ZITADEL_SERVICE_USER_TOKEN: ecs.Secret.fromSecretsManager(
          this.adminPatSecret
        ),
      },
      logging: ecs.LogDrivers.awsLogs({
        logGroup: loginLogGroup,
        streamPrefix: "zitadel-login",
      }),
      // No container health check — rely on ALB target group health check
      // (zitadel-login image has no wget/curl/shell for CMD-SHELL checks)
    });

    const loginService = new ecs.FargateService(this, "LoginService", {
      serviceName: `${prefix}-zitadel-login`,
      cluster,
      taskDefinition: loginTaskDef,
      desiredCount: config.zitadelDesiredCount,
      securityGroups: [ecsSecurityGroup],
      vpcSubnets: { subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS },
      assignPublicIp: false,
      circuitBreaker: { enable: true, rollback: true },
      enableExecuteCommand: true,
    });

    loginService.attachToApplicationTargetGroup(
      props.zitadelLoginTargetGroup as elbv2.ApplicationTargetGroup
    );

    // -------------------------------------------------------------------
    // Outputs
    // -------------------------------------------------------------------
    new cdk.CfnOutput(this, "AdminPasswordSecretArn", {
      value: adminPasswordSecret.secretArn,
      description: "ZITADEL admin human user password",
    });
    new cdk.CfnOutput(this, "AdminPatSecretArn", {
      value: this.adminPatSecret.secretArn,
      description: "Populate with admin PAT after init",
    });
    new cdk.CfnOutput(this, "ProjectIdSecretArn", {
      value: this.projectIdSecret.secretArn,
      description: "Populate with project ID after init",
    });
    new cdk.CfnOutput(this, "ApiClientIdSecretArn", {
      value: this.apiClientIdSecret.secretArn,
      description: "Populate with API client ID after init",
    });
    new cdk.CfnOutput(this, "ApiClientSecretSecretArn", {
      value: this.apiClientSecretSecret.secretArn,
      description: "Populate with API client secret after init",
    });
    new cdk.CfnOutput(this, "DashboardClientIdSecretArn", {
      value: dashboardClientIdSecret.secretArn,
      description: "Populate with dashboard OIDC client ID after init",
    });

    // -------------------------------------------------------------------
    // Tags
    // -------------------------------------------------------------------
    cdk.Tags.of(this).add("Environment", config.name);
    Object.entries(TAGS).forEach(([k, v]) => cdk.Tags.of(this).add(k, v));
  }
}
