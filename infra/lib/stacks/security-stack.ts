import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as kms from 'aws-cdk-lib/aws-kms';
import { EnvironmentConfig, ssmPath, SSM_PARAMS } from '../config';

export interface SecurityStackProps extends cdk.StackProps {
  readonly config: EnvironmentConfig;
  readonly vpc: ec2.IVpc;
}

/**
 * KMS key for envelope encryption + IAM roles for ECS and EC2.
 *
 * All secrets live in SSM Parameter Store (SecureString).
 * The CDK does NOT create the parameters — populate them before first deploy:
 *   aws ssm put-parameter --name "/llmvault/prod/jwt-signing-key" \
 *       --value "..." --type SecureString
 */
export class SecurityStack extends cdk.Stack {
  public readonly kmsKey: kms.IKey;
  public readonly taskRole: iam.Role;
  public readonly executionRole: iam.Role;
  public readonly instanceProfile: iam.InstanceProfile;

  constructor(scope: Construct, id: string, props: SecurityStackProps) {
    super(scope, id, props);

    const { config } = props;
    const env = config.envName;

    // --- KMS key (envelope encryption for credentials + RDS + ElastiCache) ---

    this.kmsKey = new kms.Key(this, 'KmsKey', {
      alias: `alias/llmvault/${env}`,
      description: `LLMVault ${env} envelope encryption key`,
      enableKeyRotation: true,
      removalPolicy: config.deletionProtection
        ? cdk.RemovalPolicy.RETAIN
        : cdk.RemovalPolicy.DESTROY,
    });

    // --- SSM parameter ARN pattern for IAM policies ---

    const ssmArn = `arn:aws:ssm:${this.region}:${this.account}:parameter/llmvault/${env}/*`;
    const secretsManagerArn = `arn:aws:secretsmanager:${this.region}:${this.account}:secret:/llmvault/${env}/*`;

    // --- ECS execution role (pull images, read secrets at task startup) ---

    this.executionRole = new iam.Role(this, 'ExecutionRole', {
      roleName: `llmvault-${env}-ecs-execution`,
      assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com'),
    });

    this.executionRole.addManagedPolicy(
      iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AmazonECSTaskExecutionRolePolicy'),
    );

    this.executionRole.addToPolicy(new iam.PolicyStatement({
      sid: 'ReadSSMSecrets',
      effect: iam.Effect.ALLOW,
      actions: ['ssm:GetParameters', 'ssm:GetParameter'],
      resources: [ssmArn],
    }));

    this.executionRole.addToPolicy(new iam.PolicyStatement({
      sid: 'ReadSecretsManager',
      effect: iam.Effect.ALLOW,
      actions: ['secretsmanager:GetSecretValue'],
      resources: [secretsManagerArn],
    }));

    this.executionRole.addToPolicy(new iam.PolicyStatement({
      sid: 'DecryptSecrets',
      effect: iam.Effect.ALLOW,
      actions: ['kms:Decrypt'],
      resources: [this.kmsKey.keyArn],
    }));

    // --- ECS task role (application-level permissions) ---

    this.taskRole = new iam.Role(this, 'TaskRole', {
      roleName: `llmvault-${env}-ecs-task`,
      assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com'),
    });

    this.taskRole.addToPolicy(new iam.PolicyStatement({
      sid: 'KMSEnvelopeEncryption',
      effect: iam.Effect.ALLOW,
      actions: ['kms:Encrypt', 'kms:Decrypt', 'kms:GenerateDataKey', 'kms:DescribeKey'],
      resources: [this.kmsKey.keyArn],
    }));

    this.taskRole.addToPolicy(new iam.PolicyStatement({
      sid: 'ECSExecForDebugging',
      effect: iam.Effect.ALLOW,
      actions: [
        'ssmmessages:CreateControlChannel',
        'ssmmessages:CreateDataChannel',
        'ssmmessages:OpenControlChannel',
        'ssmmessages:OpenDataChannel',
      ],
      resources: ['*'],
    }));

    // --- EC2 instance role (staging — same permissions + SSM Session Manager) ---

    const instanceRole = new iam.Role(this, 'InstanceRole', {
      roleName: `llmvault-${env}-ec2`,
      assumedBy: new iam.ServicePrincipal('ec2.amazonaws.com'),
    });

    instanceRole.addManagedPolicy(
      iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonSSMManagedInstanceCore'),
    );

    instanceRole.addToPolicy(new iam.PolicyStatement({
      sid: 'KMSEnvelopeEncryption',
      effect: iam.Effect.ALLOW,
      actions: ['kms:Encrypt', 'kms:Decrypt', 'kms:GenerateDataKey', 'kms:DescribeKey'],
      resources: [this.kmsKey.keyArn],
    }));

    instanceRole.addToPolicy(new iam.PolicyStatement({
      sid: 'ReadSSMSecrets',
      effect: iam.Effect.ALLOW,
      actions: ['ssm:GetParameters', 'ssm:GetParameter'],
      resources: [ssmArn],
    }));

    instanceRole.addToPolicy(new iam.PolicyStatement({
      sid: 'ReadSecretsManager',
      effect: iam.Effect.ALLOW,
      actions: ['secretsmanager:GetSecretValue'],
      resources: [secretsManagerArn],
    }));

    instanceRole.addToPolicy(new iam.PolicyStatement({
      sid: 'ECRPull',
      effect: iam.Effect.ALLOW,
      actions: [
        'ecr:GetAuthorizationToken',
        'ecr:BatchCheckLayerAvailability',
        'ecr:GetDownloadUrlForLayer',
        'ecr:BatchGetImage',
      ],
      resources: ['*'],
    }));

    instanceRole.addToPolicy(new iam.PolicyStatement({
      sid: 'CloudWatchLogs',
      effect: iam.Effect.ALLOW,
      actions: ['logs:CreateLogGroup', 'logs:CreateLogStream', 'logs:PutLogEvents'],
      resources: [`arn:aws:logs:${this.region}:${this.account}:log-group:/llmvault/*`],
    }));

    this.instanceProfile = new iam.InstanceProfile(this, 'InstanceProfile', {
      instanceProfileName: `llmvault-${env}-ec2`,
      role: instanceRole,
    });
  }
}
