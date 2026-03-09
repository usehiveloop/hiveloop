import { Construct } from 'constructs';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as elasticache from 'aws-cdk-lib/aws-elasticache';

export interface ElastiCacheRedisProps {
  readonly vpc: ec2.IVpc;
  readonly securityGroup: ec2.ISecurityGroup;
  readonly nodeType: string;
  readonly envName: string;
  readonly authTokenSsmPath: string;
}

export class ElastiCacheRedis extends Construct {
  public readonly endpoint: string;
  public readonly port: string;

  constructor(scope: Construct, id: string, props: ElastiCacheRedisProps) {
    super(scope, id);

    const subnetGroup = new elasticache.CfnSubnetGroup(this, 'SubnetGroup', {
      cacheSubnetGroupName: `llmvault-${props.envName}-redis`,
      description: `LLMVault ${props.envName} Redis subnets`,
      subnetIds: props.vpc.selectSubnets({
        subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS,
      }).subnetIds,
    });

    const replicationGroup = new elasticache.CfnReplicationGroup(this, 'Redis', {
      replicationGroupDescription: `LLMVault ${props.envName} Redis`,
      engine: 'redis',
      cacheNodeType: props.nodeType,
      numNodeGroups: 1,
      replicasPerNodeGroup: 0,
      cacheSubnetGroupName: subnetGroup.ref,
      securityGroupIds: [props.securityGroup.securityGroupId],
      transitEncryptionEnabled: true,
      atRestEncryptionEnabled: true,
      authToken: `{{resolve:ssm-secure:${props.authTokenSsmPath}}}`,
      automaticFailoverEnabled: false,
      multiAzEnabled: false,
      port: 6379,
    });

    replicationGroup.addDependency(subnetGroup);

    this.endpoint = replicationGroup.attrPrimaryEndPointAddress;
    this.port = replicationGroup.attrPrimaryEndPointPort;
  }
}
