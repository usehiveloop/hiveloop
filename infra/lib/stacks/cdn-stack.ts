import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as acm from 'aws-cdk-lib/aws-certificatemanager';
import * as cloudfront from 'aws-cdk-lib/aws-cloudfront';
import * as origins from 'aws-cdk-lib/aws-cloudfront-origins';
import * as s3 from 'aws-cdk-lib/aws-s3';
import { EnvironmentConfig } from '../config';

export interface CdnStackProps extends cdk.StackProps {
  readonly config: EnvironmentConfig;
}

/**
 * S3 + CloudFront for the Connect SPA (static Vite build).
 *
 * The ACM certificate must be in us-east-1 for CloudFront. If deploying to
 * another region, create the certificate manually in us-east-1 first.
 *
 * DNS is managed in Cloudflare. After deploy, create a CNAME record:
 *   connect.llmvault.dev → <CloudFront distribution domain>
 *
 * Deploy new builds:
 *   aws s3 sync apps/connect/dist/ s3://<bucket> --delete
 *   aws cloudfront create-invalidation --distribution-id <id> --paths "/*"
 */
export class CdnStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: CdnStackProps) {
    super(scope, id, props);

    const { config } = props;

    // ACM cert for CloudFront — must be in us-east-1.
    // On first deploy, CloudFormation pauses for DNS validation.
    // Add the CNAME records from the ACM console into Cloudflare.
    const cert = new acm.Certificate(this, 'Cert', {
      domainName: config.domains.connect,
      validation: acm.CertificateValidation.fromDns(),
    });

    const bucket = new s3.Bucket(this, 'Bucket', {
      bucketName: `llmvault-${config.envName}-connect`,
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
      enforceSSL: true,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
      autoDeleteObjects: true,
    });

    const oac = new cloudfront.S3OriginAccessControl(this, 'OAC', {
      signing: cloudfront.Signing.SIGV4_ALWAYS,
    });

    const distribution = new cloudfront.Distribution(this, 'Distribution', {
      domainNames: [config.domains.connect],
      certificate: cert,
      defaultBehavior: {
        origin: origins.S3BucketOrigin.withOriginAccessControl(bucket, {
          originAccessControl: oac,
        }),
        viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
        cachePolicy: cloudfront.CachePolicy.CACHING_OPTIMIZED,
        responseHeadersPolicy: cloudfront.ResponseHeadersPolicy.SECURITY_HEADERS,
      },
      defaultRootObject: 'index.html',
      errorResponses: [
        {
          httpStatus: 403,
          responseHttpStatus: 200,
          responsePagePath: '/index.html',
          ttl: cdk.Duration.seconds(0),
        },
        {
          httpStatus: 404,
          responseHttpStatus: 200,
          responsePagePath: '/index.html',
          ttl: cdk.Duration.seconds(0),
        },
      ],
      httpVersion: cloudfront.HttpVersion.HTTP2_AND_3,
      priceClass: cloudfront.PriceClass.PRICE_CLASS_100,
    });

    // --- Outputs ---

    new cdk.CfnOutput(this, 'BucketName', { value: bucket.bucketName });
    new cdk.CfnOutput(this, 'DistributionId', { value: distribution.distributionId });
    new cdk.CfnOutput(this, 'DistributionDomain', {
      value: distribution.distributionDomainName,
      description: 'CloudFront domain — create a CNAME in Cloudflare pointing to this',
    });
    new cdk.CfnOutput(this, 'CloudflareDnsRecord', {
      value: `${config.domains.connect} → CNAME → ${distribution.distributionDomainName}`,
      description: 'DNS record to add in Cloudflare',
    });
  }
}
