import { Construct } from 'constructs';
import * as cdk from 'aws-cdk-lib';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as kms from 'aws-cdk-lib/aws-kms';
import * as rds from 'aws-cdk-lib/aws-rds';

export interface RdsPostgresProps {
  readonly vpc: ec2.IVpc;
  readonly securityGroup: ec2.ISecurityGroup;
  readonly kmsKey: kms.IKey;
  readonly instanceClass: string;
  readonly allocatedStorageGb: number;
  readonly multiAz: boolean;
  readonly deletionProtection: boolean;
  readonly envName: string;
}

export class RdsPostgres extends Construct {
  public readonly instance: rds.DatabaseInstance;
  public readonly secret: cdk.aws_secretsmanager.ISecret;

  constructor(scope: Construct, id: string, props: RdsPostgresProps) {
    super(scope, id);

    const subnetGroup = new rds.SubnetGroup(this, 'SubnetGroup', {
      vpc: props.vpc,
      description: `LLMVault ${props.envName} DB subnets`,
      vpcSubnets: { subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS },
    });

    this.instance = new rds.DatabaseInstance(this, 'Instance', {
      engine: rds.DatabaseInstanceEngine.postgres({
        version: rds.PostgresEngineVersion.VER_17,
      }),
      instanceType: new ec2.InstanceType(props.instanceClass.replace('db.', '')),
      vpc: props.vpc,
      subnetGroup,
      securityGroups: [props.securityGroup],
      databaseName: 'llmvault',
      credentials: rds.Credentials.fromGeneratedSecret('llmvault', {
        secretName: `/llmvault/${props.envName}/rds-credentials`,
        encryptionKey: props.kmsKey,
      }),
      storageEncrypted: true,
      storageEncryptionKey: props.kmsKey,
      allocatedStorage: props.allocatedStorageGb,
      storageType: rds.StorageType.GP3,
      multiAz: props.multiAz,
      deletionProtection: props.deletionProtection,
      removalPolicy: props.deletionProtection
        ? cdk.RemovalPolicy.RETAIN
        : cdk.RemovalPolicy.DESTROY,
      backupRetention: cdk.Duration.days(props.deletionProtection ? 7 : 1),
      monitoringInterval: cdk.Duration.seconds(60),
      enablePerformanceInsights: true,
      performanceInsightEncryptionKey: props.kmsKey,
    });

    this.secret = this.instance.secret!;
  }
}
