import { Construct } from 'constructs';
import * as cdk from 'aws-cdk-lib';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as ecs from 'aws-cdk-lib/aws-ecs';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as logs from 'aws-cdk-lib/aws-logs';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';

export interface FargateServiceProps {
  readonly cluster: ecs.ICluster;
  readonly serviceName: string;
  readonly image: ecs.ContainerImage;
  readonly containerPort: number;
  readonly cpu: number;
  readonly memoryLimitMiB: number;
  readonly desiredCount: number;
  readonly healthCheckPath: string;
  readonly environment: Record<string, string>;
  readonly secrets: Record<string, ecs.Secret>;
  readonly taskRole: iam.IRole;
  readonly executionRole: iam.IRole;
  readonly securityGroups: ec2.ISecurityGroup[];
  readonly subnets: ec2.SubnetSelection;
  readonly command?: string[];
}

export class FargateService extends Construct {
  public readonly service: ecs.FargateService;
  public readonly targetGroup: elbv2.ApplicationTargetGroup;

  constructor(scope: Construct, id: string, props: FargateServiceProps) {
    super(scope, id);

    const logGroup = new logs.LogGroup(this, 'Logs', {
      logGroupName: `/llmvault/${props.serviceName}`,
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    const taskDef = new ecs.FargateTaskDefinition(this, 'Task', {
      cpu: props.cpu,
      memoryLimitMiB: props.memoryLimitMiB,
      taskRole: props.taskRole,
      executionRole: props.executionRole,
      runtimePlatform: {
        cpuArchitecture: ecs.CpuArchitecture.ARM64,
        operatingSystemFamily: ecs.OperatingSystemFamily.LINUX,
      },
    });

    taskDef.addContainer('app', {
      image: props.image,
      containerName: props.serviceName,
      portMappings: [{ containerPort: props.containerPort }],
      environment: props.environment,
      secrets: props.secrets,
      command: props.command,
      logging: ecs.LogDrivers.awsLogs({ logGroup, streamPrefix: props.serviceName }),
      healthCheck: {
        command: ['CMD-SHELL', `wget -qO- http://localhost:${props.containerPort}${props.healthCheckPath} || exit 1`],
        interval: cdk.Duration.seconds(15),
        timeout: cdk.Duration.seconds(5),
        retries: 3,
        startPeriod: cdk.Duration.seconds(60),
      },
    });

    this.service = new ecs.FargateService(this, 'Service', {
      cluster: props.cluster,
      taskDefinition: taskDef,
      desiredCount: props.desiredCount,
      securityGroups: props.securityGroups,
      vpcSubnets: props.subnets,
      assignPublicIp: true,
      enableExecuteCommand: true,
      circuitBreaker: { rollback: true },
      minHealthyPercent: 100,
      maxHealthyPercent: 200,
    });

    this.targetGroup = new elbv2.ApplicationTargetGroup(this, 'TG', {
      vpc: props.cluster.vpc,
      port: props.containerPort,
      protocol: elbv2.ApplicationProtocol.HTTP,
      targetType: elbv2.TargetType.IP,
      healthCheck: {
        path: props.healthCheckPath,
        interval: cdk.Duration.seconds(30),
        healthyThresholdCount: 2,
        unhealthyThresholdCount: 3,
      },
      deregistrationDelay: cdk.Duration.seconds(15),
    });

    this.targetGroup.addTarget(this.service);
  }
}
