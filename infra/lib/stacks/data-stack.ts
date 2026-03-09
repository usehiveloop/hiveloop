import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as kms from 'aws-cdk-lib/aws-kms';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as cr from 'aws-cdk-lib/custom-resources';
import { EnvironmentConfig, ssmPath, SSM_PARAMS } from '../config';
import { SecurityGroups } from './network-stack';
import { RdsPostgres } from '../constructs/rds-postgres';
import { ElastiCacheRedis } from '../constructs/elasticache-redis';

export interface DataStackProps extends cdk.StackProps {
  readonly config: EnvironmentConfig;
  readonly vpc: ec2.IVpc;
  readonly securityGroups: SecurityGroups;
  readonly kmsKey: kms.IKey;
}

/**
 * Managed data stores — RDS Postgres + ElastiCache Redis.
 * Only created in 'fargate' phase.
 *
 * A Lambda custom resource automatically constructs DATABASE_URL from the
 * RDS credentials and stores it in SSM so the Go server can consume it
 * without any code changes.
 */
export class DataStack extends cdk.Stack {
  public readonly rds: RdsPostgres;
  public readonly redis: ElastiCacheRedis;

  constructor(scope: Construct, id: string, props: DataStackProps) {
    super(scope, id, props);

    const { config } = props;

    // --- RDS Postgres ---

    this.rds = new RdsPostgres(this, 'Postgres', {
      vpc: props.vpc,
      securityGroup: props.securityGroups.rds,
      kmsKey: props.kmsKey,
      instanceClass: config.rdsInstanceClass,
      allocatedStorageGb: config.rdsAllocatedStorageGb,
      multiAz: config.rdsMultiAz,
      deletionProtection: config.deletionProtection,
      envName: config.envName,
    });

    // --- ElastiCache Redis ---

    this.redis = new ElastiCacheRedis(this, 'Redis', {
      vpc: props.vpc,
      securityGroup: props.securityGroups.redis,
      nodeType: config.redisNodeType,
      envName: config.envName,
      authTokenSsmPath: ssmPath(config.envName, SSM_PARAMS.redisPassword),
    });

    // --- Custom resource: construct DATABASE_URL and store in SSM ---

    this.createDatabaseUrlResource(config, props.kmsKey);
  }

  private createDatabaseUrlResource(config: EnvironmentConfig, kmsKey: kms.IKey) {
    const fn = new lambda.Function(this, 'DbUrlBuilder', {
      functionName: `llmvault-${config.envName}-db-url-builder`,
      runtime: lambda.Runtime.PYTHON_3_12,
      handler: 'index.handler',
      timeout: cdk.Duration.seconds(30),
      code: lambda.Code.fromInline(`
import boto3
import json

def handler(event, context):
    if event["RequestType"] == "Delete":
        return {"PhysicalResourceId": event.get("PhysicalResourceId", "db-url")}

    props = event["ResourceProperties"]
    sm = boto3.client("secretsmanager")
    raw = sm.get_secret_value(SecretId=props["SecretArn"])["SecretString"]
    creds = json.loads(raw)

    url = (
        f"postgres://{creds['username']}:{creds['password']}"
        f"@{creds['host']}:{creds['port']}/{props['DbName']}?sslmode=require"
    )

    ssm = boto3.client("ssm")
    ssm.put_parameter(
        Name=props["SsmPath"],
        Value=url,
        Type="SecureString",
        KeyId=props["KmsKeyId"],
        Overwrite=True,
    )

    return {"PhysicalResourceId": props["SsmPath"]}
      `.trim()),
    });

    fn.addToRolePolicy(new iam.PolicyStatement({
      actions: ['secretsmanager:GetSecretValue'],
      resources: [this.rds.secret.secretArn],
    }));

    fn.addToRolePolicy(new iam.PolicyStatement({
      actions: ['ssm:PutParameter'],
      resources: [
        `arn:aws:ssm:${this.region}:${this.account}:parameter${ssmPath(config.envName, SSM_PARAMS.databaseUrl)}`,
      ],
    }));

    fn.addToRolePolicy(new iam.PolicyStatement({
      actions: ['kms:Encrypt', 'kms:Decrypt', 'kms:GenerateDataKey'],
      resources: [kmsKey.keyArn],
    }));

    const provider = new cr.Provider(this, 'DbUrlProvider', {
      onEventHandler: fn,
    });

    new cdk.CustomResource(this, 'DbUrlResource', {
      serviceToken: provider.serviceToken,
      properties: {
        SecretArn: this.rds.secret.secretArn,
        SsmPath: ssmPath(config.envName, SSM_PARAMS.databaseUrl),
        DbName: 'llmvault',
        KmsKeyId: kmsKey.keyId,
        // Force update when RDS instance changes (new endpoint)
        RdsHost: this.rds.instance.dbInstanceEndpointAddress,
      },
    });
  }
}
