import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as codebuild from 'aws-cdk-lib/aws-codebuild';
import * as iam from 'aws-cdk-lib/aws-iam';
import { EnvironmentConfig, ssmPath, SSM_PARAMS } from '../config';
import { SharedStack } from './shared-stack';

export interface BuildStackProps extends cdk.StackProps {
  readonly shared: SharedStack;
  readonly production: EnvironmentConfig;
  readonly staging: EnvironmentConfig;
  readonly githubOwner: string;
  readonly githubRepo: string;
}

/**
 * AWS CodeBuild project for building Docker images and deploying.
 *
 * Uses a GitHub webhook to trigger builds on push/tag. ARM64 compute for
 * native Graviton image builds (no QEMU emulation). Docker layer caching
 * for fast rebuilds.
 *
 * Trigger rules:
 *   push to main    → build images only
 *   push to develop → build + deploy staging
 *   tag v*          → build + deploy staging + deploy production
 *
 * One-time setup required:
 *   aws codebuild import-source-credentials \
 *     --server-type GITHUB \
 *     --auth-type PERSONAL_ACCESS_TOKEN \
 *     --token <github-pat>
 */
export class BuildStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: BuildStackProps) {
    super(scope, id, props);

    const { production: prod, staging: stg } = props;
    const ecrRegistry = `${this.account}.dkr.ecr.${this.region}.amazonaws.com`;

    const project = new codebuild.Project(this, 'Build', {
      projectName: 'llmvault-build',
      description: 'Build LLMVault Docker images and deploy to ECS/EC2',

      source: codebuild.Source.gitHub({
        owner: props.githubOwner,
        repo: props.githubRepo,
        webhook: true,
        webhookFilters: [
          codebuild.FilterGroup
            .inEventOf(codebuild.EventAction.PUSH)
            .andBranchIs('main'),
          codebuild.FilterGroup
            .inEventOf(codebuild.EventAction.PUSH)
            .andBranchIs('develop'),
          codebuild.FilterGroup
            .inEventOf(codebuild.EventAction.PUSH)
            .andTagIs('v.*'),
        ],
      }),

      buildSpec: codebuild.BuildSpec.fromSourceFilename('buildspec.yml'),

      environment: {
        buildImage: codebuild.LinuxArmBuildImage.AMAZON_LINUX_2_STANDARD_3_0,
        computeType: codebuild.ComputeType.SMALL,
        privileged: true, // required for Docker builds
      },

      environmentVariables: {
        // --- ECR ---
        ECR_REGISTRY: { value: ecrRegistry },

        // --- Web build args (baked into Next.js at build time) ---
        NEXT_PUBLIC_API_URL: { value: `https://${prod.domains.api}` },
        NEXT_PUBLIC_ZITADEL_ISSUER: { value: `https://${prod.domains.auth}` },
        NEXT_PUBLIC_CONNECT_URL: { value: `https://${prod.domains.connect}` },
        NEXT_PUBLIC_ZITADEL_CLIENT_ID: {
          type: codebuild.BuildEnvironmentVariableType.PARAMETER_STORE,
          value: ssmPath(prod.envName, SSM_PARAMS.zitadelDashboardClientId),
        },

        // --- Staging deployment ---
        STG_DEPLOY_MODE: { value: stg.phase },
        STG_CLUSTER: { value: `llmvault-${stg.envName}` },
        STG_INSTANCE_TAG: { value: `llmvault-${stg.envName}` },
        STG_CONNECT_BUCKET: { value: `llmvault-${stg.envName}-connect` },
        STG_CDN_STACK: { value: `LLMVault-${stg.envName}-Cdn` },

        // --- Production deployment ---
        PROD_CLUSTER: { value: `llmvault-${prod.envName}` },
        PROD_CONNECT_BUCKET: { value: `llmvault-${prod.envName}-connect` },
        PROD_CDN_STACK: { value: `LLMVault-${prod.envName}-Cdn` },
      },

      timeout: cdk.Duration.minutes(30),
      concurrentBuildLimit: 1,
      cache: codebuild.Cache.local(
        codebuild.LocalCacheMode.DOCKER_LAYER,
        codebuild.LocalCacheMode.CUSTOM,
      ),
    });

    // --- IAM permissions (least-privilege) ---

    // ECR push
    props.shared.apiRepo.grantPullPush(project);
    props.shared.webRepo.grantPullPush(project);

    // SSM read (NEXT_PUBLIC_ZITADEL_CLIENT_ID from parameter store)
    project.addToRolePolicy(new iam.PolicyStatement({
      sid: 'SSMReadParams',
      actions: ['ssm:GetParameter', 'ssm:GetParameters'],
      resources: [
        `arn:aws:ssm:${this.region}:${this.account}:parameter/llmvault/${prod.envName}/*`,
        `arn:aws:ssm:${this.region}:${this.account}:parameter/llmvault/${stg.envName}/*`,
      ],
    }));

    // S3 sync (Connect SPA buckets)
    project.addToRolePolicy(new iam.PolicyStatement({
      sid: 'S3ConnectSync',
      actions: ['s3:PutObject', 's3:DeleteObject', 's3:ListBucket', 's3:GetObject'],
      resources: [
        `arn:aws:s3:::llmvault-${prod.envName}-connect`,
        `arn:aws:s3:::llmvault-${prod.envName}-connect/*`,
        `arn:aws:s3:::llmvault-${stg.envName}-connect`,
        `arn:aws:s3:::llmvault-${stg.envName}-connect/*`,
      ],
    }));

    // ECS deploy (both environments — staging may upgrade to fargate)
    project.addToRolePolicy(new iam.PolicyStatement({
      sid: 'ECSForceDeploy',
      actions: ['ecs:UpdateService', 'ecs:DescribeServices'],
      resources: [
        `arn:aws:ecs:${this.region}:${this.account}:service/llmvault-${prod.envName}/*`,
        `arn:aws:ecs:${this.region}:${this.account}:service/llmvault-${stg.envName}/*`,
      ],
    }));

    // SSM RunCommand (staging EC2 deploy)
    project.addToRolePolicy(new iam.PolicyStatement({
      sid: 'SSMRunCommand',
      actions: ['ssm:SendCommand'],
      resources: [
        `arn:aws:ssm:${this.region}::document/AWS-RunShellScript`,
        `arn:aws:ec2:${this.region}:${this.account}:instance/*`,
      ],
    }));

    // CloudFormation describe (to look up CloudFront distribution ID)
    project.addToRolePolicy(new iam.PolicyStatement({
      sid: 'CloudFormationDescribe',
      actions: ['cloudformation:DescribeStacks'],
      resources: [
        `arn:aws:cloudformation:${this.region}:${this.account}:stack/LLMVault-*/*`,
      ],
    }));

    // CloudFront invalidation
    project.addToRolePolicy(new iam.PolicyStatement({
      sid: 'CloudFrontInvalidation',
      actions: ['cloudfront:CreateInvalidation'],
      resources: [`arn:aws:cloudfront::${this.account}:distribution/*`],
    }));

    // --- Outputs ---

    new cdk.CfnOutput(this, 'ProjectName', { value: project.projectName! });
    new cdk.CfnOutput(this, 'ProjectArn', { value: project.projectArn });
  }
}
