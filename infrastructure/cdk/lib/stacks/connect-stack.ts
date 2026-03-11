import * as cdk from "aws-cdk-lib";
import * as s3 from "aws-cdk-lib/aws-s3";
import * as cloudfront from "aws-cdk-lib/aws-cloudfront";
import * as origins from "aws-cdk-lib/aws-cloudfront-origins";
import * as acm from "aws-cdk-lib/aws-certificatemanager";
import * as iam from "aws-cdk-lib/aws-iam";
import { Construct } from "constructs";
import { EnvironmentConfig, subdomain } from "../config/environments";
import { PROJECT_NAME, TAGS } from "../config/constants";


export interface ConnectStackProps extends cdk.StackProps {
  config: EnvironmentConfig;
  /** Optional ACM certificate ARN for custom domain (must be in us-east-1) */
  certificateArn?: string;
}

export class ConnectStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: ConnectStackProps) {
    super(scope, id, props);

    const { config, certificateArn } = props;
    const prefix = `${PROJECT_NAME}-${config.name}`;
    const domain = subdomain(config, "connect");
    
    // Import certificate if ARN provided (must be in us-east-1 for CloudFront)
    const certificate = certificateArn 
      ? acm.Certificate.fromCertificateArn(this, "ConnectCertificate", certificateArn)
      : undefined;

    // -------------------------------------------------------------------
    // S3 Bucket for static assets
    // -------------------------------------------------------------------
    const bucket = new s3.Bucket(this, "ConnectBucket", {
      bucketName: `${prefix}-connect-assets`,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
      autoDeleteObjects: true,
      publicReadAccess: false, // CloudFront will access it via OAI
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
    });

    // -------------------------------------------------------------------
    // Origin Access Identity for CloudFront to access S3
    // -------------------------------------------------------------------
    const originAccessIdentity = new cloudfront.OriginAccessIdentity(
      this,
      "ConnectOAI",
      {
        comment: `OAI for ${prefix} connect app`,
      }
    );

    // Grant CloudFront read access to the bucket
    bucket.grantRead(originAccessIdentity);

    // -------------------------------------------------------------------
    // CloudFront Distribution
    // -------------------------------------------------------------------
    // NOTE: For custom domain, you need to:
    // 1. Create ACM certificate in us-east-1 manually or via separate stack
    // 2. Validate it via DNS in Cloudflare
    // 3. Pass the certificate ARN here
    const distribution = new cloudfront.Distribution(this, "ConnectDistribution", {
      defaultRootObject: "index.html",
      domainNames: certificate ? [domain] : undefined,
      certificate: certificate,
      priceClass: cloudfront.PriceClass.PRICE_CLASS_100, // North America & Europe
      
      defaultBehavior: {
        origin: new origins.S3Origin(bucket, {
          originAccessIdentity,
        }),
        viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
        cachePolicy: cloudfront.CachePolicy.CACHING_OPTIMIZED,
        allowedMethods: cloudfront.AllowedMethods.ALLOW_GET_HEAD,
        cachedMethods: cloudfront.CachedMethods.CACHE_GET_HEAD,
        compress: true,
      },
      
      // SPA routing: return index.html for 404s
      errorResponses: [
        {
          httpStatus: 403,
          responseHttpStatus: 200,
          responsePagePath: "/index.html",
          ttl: cdk.Duration.minutes(0),
        },
        {
          httpStatus: 404,
          responseHttpStatus: 200,
          responsePagePath: "/index.html",
          ttl: cdk.Duration.minutes(0),
        },
      ],
    });

    // -------------------------------------------------------------------
    // Outputs
    // -------------------------------------------------------------------
    new cdk.CfnOutput(this, "ConnectDomain", {
      value: domain,
      description: "Connect app domain (add CNAME to Cloudflare after adding custom domain)",
    });

    new cdk.CfnOutput(this, "SetupInstructions", {
      value: "1. Create ACM cert in us-east-1 for " + domain + " | 2. Add CNAME in Cloudflare | 3. Update distribution with certificate ARN",
      description: "Steps to enable custom domain",
    });

    new cdk.CfnOutput(this, "CloudFrontDomain", {
      value: distribution.distributionDomainName,
      description:
        "CloudFront distribution domain - create CNAME in Cloudflare pointing to this",
    });

    new cdk.CfnOutput(this, "S3BucketName", {
      value: bucket.bucketName,
      description: "S3 bucket for Connect static assets",
    });

    new cdk.CfnOutput(this, "DistributionId", {
      value: distribution.distributionId,
      description: "CloudFront Distribution ID for cache invalidation",
    });

    // -------------------------------------------------------------------
    // IAM User for deployments (optional - can also use existing credentials)
    // -------------------------------------------------------------------
    const deployUser = new iam.User(this, "ConnectDeployUser", {
      userName: `${prefix}-connect-deployer`,
    });
    bucket.grantReadWrite(deployUser);
    
    // Policy to allow CloudFront invalidation
    deployUser.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "cloudfront:CreateInvalidation",
          "cloudfront:GetInvalidation",
          "cloudfront:ListInvalidations",
        ],
        resources: [`arn:aws:cloudfront::${this.account}:distribution/${distribution.distributionId}`],
      })
    );

    new cdk.CfnOutput(this, "DeployUserArn", {
      value: deployUser.userArn,
      description: "IAM user ARN for deployment credentials",
    });

    // -------------------------------------------------------------------
    // Tags
    // -------------------------------------------------------------------
    cdk.Tags.of(this).add("Environment", config.name);
    cdk.Tags.of(this).add("Service", "connect");
    Object.entries(TAGS).forEach(([k, v]) => cdk.Tags.of(this).add(k, v));
  }
}
