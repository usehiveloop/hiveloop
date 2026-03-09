import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as ecr from 'aws-cdk-lib/aws-ecr';

/**
 * Account-level resources shared across all environments.
 * Deploy once: `cdk deploy LLMVault-Shared`
 */
export class SharedStack extends cdk.Stack {
  public readonly apiRepo: ecr.IRepository;
  public readonly webRepo: ecr.IRepository;

  constructor(scope: Construct, id: string, props: cdk.StackProps) {
    super(scope, id, props);

    this.apiRepo = this.createRepo('api');
    this.webRepo = this.createRepo('web');
  }

  private createRepo(name: string): ecr.Repository {
    return new ecr.Repository(this, `${name}Repo`, {
      repositoryName: `llmvault/${name}`,
      imageScanOnPush: true,
      lifecycleRules: [
        {
          description: 'Keep last 20 images',
          maxImageCount: 20,
          rulePriority: 1,
        },
      ],
      removalPolicy: cdk.RemovalPolicy.RETAIN,
    });
  }
}
