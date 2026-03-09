import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import { EnvironmentConfig } from '../config';

export interface SecurityGroups {
  readonly alb: ec2.ISecurityGroup;
  readonly api: ec2.ISecurityGroup;
  readonly web: ec2.ISecurityGroup;
  readonly zitadel: ec2.ISecurityGroup;
  readonly rds: ec2.ISecurityGroup;
  readonly redis: ec2.ISecurityGroup;
  readonly ec2: ec2.ISecurityGroup;
}

export interface NetworkStackProps extends cdk.StackProps {
  readonly config: EnvironmentConfig;
}

/**
 * VPC with 2 AZs and all security groups.
 *
 * Uses public subnets for Fargate tasks (assignPublicIp) to avoid NAT Gateway
 * costs ($33/mo). Security groups restrict ingress to ALB only.
 * Private subnets hold RDS and ElastiCache — no internet route needed.
 */
export class NetworkStack extends cdk.Stack {
  public readonly vpc: ec2.IVpc;
  public readonly securityGroups: SecurityGroups;

  constructor(scope: Construct, id: string, props: NetworkStackProps) {
    super(scope, id, props);

    const { config } = props;

    const vpc = new ec2.Vpc(this, 'Vpc', {
      vpcName: `llmvault-${config.envName}`,
      maxAzs: 2,
      natGateways: 0,
      subnetConfiguration: [
        {
          name: 'public',
          subnetType: ec2.SubnetType.PUBLIC,
          cidrMask: 24,
        },
        {
          name: 'private',
          subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS,
          cidrMask: 24,
        },
      ],
    });

    this.vpc = vpc;

    // --- Security groups ---

    const albSg = new ec2.SecurityGroup(this, 'AlbSg', {
      vpc,
      description: 'ALB — public HTTPS ingress',
      allowAllOutbound: true,
    });
    albSg.addIngressRule(ec2.Peer.anyIpv4(), ec2.Port.tcp(443), 'HTTPS');
    albSg.addIngressRule(ec2.Peer.anyIpv4(), ec2.Port.tcp(80), 'HTTP redirect');

    const apiSg = new ec2.SecurityGroup(this, 'ApiSg', {
      vpc,
      description: 'Go API — inbound from ALB only',
      allowAllOutbound: true,
    });
    apiSg.addIngressRule(albSg, ec2.Port.tcp(8080), 'ALB → API');

    const webSg = new ec2.SecurityGroup(this, 'WebSg', {
      vpc,
      description: 'Next.js — inbound from ALB only',
      allowAllOutbound: true,
    });
    webSg.addIngressRule(albSg, ec2.Port.tcp(3000), 'ALB → Web');

    const zitadelSg = new ec2.SecurityGroup(this, 'ZitadelSg', {
      vpc,
      description: 'ZITADEL — inbound from ALB + API (introspection)',
      allowAllOutbound: true,
    });
    zitadelSg.addIngressRule(albSg, ec2.Port.tcp(8080), 'ALB → ZITADEL');
    zitadelSg.addIngressRule(apiSg, ec2.Port.tcp(8080), 'API → ZITADEL (introspection)');

    const rdsSg = new ec2.SecurityGroup(this, 'RdsSg', {
      vpc,
      description: 'RDS — inbound from API + ZITADEL only',
      allowAllOutbound: false,
    });
    rdsSg.addIngressRule(apiSg, ec2.Port.tcp(5432), 'API → RDS');
    rdsSg.addIngressRule(zitadelSg, ec2.Port.tcp(5432), 'ZITADEL → RDS');

    const redisSg = new ec2.SecurityGroup(this, 'RedisSg', {
      vpc,
      description: 'Redis — inbound from API only',
      allowAllOutbound: false,
    });
    redisSg.addIngressRule(apiSg, ec2.Port.tcp(6379), 'API → Redis');

    const ec2Sg = new ec2.SecurityGroup(this, 'Ec2Sg', {
      vpc,
      description: 'EC2 single-host — public HTTPS + SSH via SSM',
      allowAllOutbound: true,
    });
    ec2Sg.addIngressRule(ec2.Peer.anyIpv4(), ec2.Port.tcp(443), 'HTTPS');
    ec2Sg.addIngressRule(ec2.Peer.anyIpv4(), ec2.Port.tcp(80), 'HTTP redirect');

    this.securityGroups = {
      alb: albSg,
      api: apiSg,
      web: webSg,
      zitadel: zitadelSg,
      rds: rdsSg,
      redis: redisSg,
      ec2: ec2Sg,
    };
  }
}
