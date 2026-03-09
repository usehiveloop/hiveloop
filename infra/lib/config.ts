/**
 * Environment configuration for LLMVault deployments.
 *
 * Edit the production/staging objects below to scale up:
 *   - phase: 'ec2' → 'fargate' to move to managed services
 *   - rdsMultiAz: true to enable database failover
 *   - tasksPerService: 2 to run across both AZs
 */

export interface FargateSizing {
  readonly cpu: number;
  readonly memory: number;
}

export interface EnvironmentConfig {
  readonly envName: string;
  readonly account: string;
  readonly region: string;

  /** 'ec2' = single Docker host, 'fargate' = managed ECS + RDS + ElastiCache */
  readonly phase: 'ec2' | 'fargate';

  readonly domains: {
    readonly root: string;
    readonly api: string;
    readonly auth: string;
    readonly connect: string;
  };

  // --- HA knobs ---
  readonly rdsMultiAz: boolean;
  readonly tasksPerService: number;

  // --- Sizing ---
  readonly fargate: {
    readonly api: FargateSizing;
    readonly web: FargateSizing;
    readonly zitadel: FargateSizing;
  };
  readonly rdsInstanceClass: string;
  readonly rdsAllocatedStorageGb: number;
  readonly redisNodeType: string;
  readonly ec2InstanceType: string;

  // --- Security ---
  readonly deletionProtection: boolean;

  // --- Images ---
  readonly zitadelImage: string;
}

// ---------------------------------------------------------------------------
// SSM parameter naming convention: /llmvault/{env}/{name}
// ---------------------------------------------------------------------------

export function ssmPath(env: string, name: string): string {
  return `/llmvault/${env}/${name}`;
}

// ---------------------------------------------------------------------------
// GitHub repository (for CodeBuild webhook)
// ---------------------------------------------------------------------------

export const github = {
  owner: 'useportal',
  repo: 'llmvault',
} as const;

export const SSM_PARAMS = {
  databaseUrl: 'database-url',
  redisPassword: 'redis-password',
  jwtSigningKey: 'jwt-signing-key',
  zitadelMasterkey: 'zitadel-masterkey',
  zitadelDbPassword: 'zitadel-db-password',
  zitadelClientId: 'zitadel-client-id',
  zitadelClientSecret: 'zitadel-client-secret',
  zitadelAdminPat: 'zitadel-admin-pat',
  zitadelProjectId: 'zitadel-project-id',
  zitadelDashboardClientId: 'zitadel-dashboard-client-id',
} as const;

// ---------------------------------------------------------------------------
// Environments — edit these to scale
// ---------------------------------------------------------------------------

export const production: EnvironmentConfig = {
  envName: 'prod',
  account: process.env.CDK_DEFAULT_ACCOUNT!,
  region: process.env.CDK_DEFAULT_REGION || 'us-east-1',
  phase: 'fargate',

  domains: {
    root: 'llmvault.dev',
    api: 'api.llmvault.dev',
    auth: 'auth.llmvault.dev',
    connect: 'connect.llmvault.dev',
  },
  rdsMultiAz: false,                  // flip to true for HA
  tasksPerService: 1,                 // bump to 2 for HA

  fargate: {
    api:     { cpu: 256, memory: 512 },
    web:     { cpu: 256, memory: 512 },
    zitadel: { cpu: 256, memory: 1024 },
  },
  rdsInstanceClass: 'db.t4g.micro',
  rdsAllocatedStorageGb: 20,
  redisNodeType: 'cache.t4g.micro',
  ec2InstanceType: 't4g.small',

  deletionProtection: true,
  zitadelImage: 'ghcr.io/zitadel/zitadel:v4.11.0',
};

export const staging: EnvironmentConfig = {
  envName: 'staging',
  account: process.env.CDK_DEFAULT_ACCOUNT!,
  region: process.env.CDK_DEFAULT_REGION || 'us-east-1',
  phase: 'ec2',

  domains: {
    root: 'dev.llmvault.dev',
    api: 'api.dev.llmvault.dev',
    auth: 'auth.dev.llmvault.dev',
    connect: 'connect.dev.llmvault.dev',
  },
  rdsMultiAz: false,
  tasksPerService: 1,

  fargate: {
    api:     { cpu: 256, memory: 512 },
    web:     { cpu: 256, memory: 512 },
    zitadel: { cpu: 256, memory: 1024 },
  },
  rdsInstanceClass: 'db.t4g.micro',
  rdsAllocatedStorageGb: 20,
  redisNodeType: 'cache.t4g.micro',
  ec2InstanceType: 't4g.small',

  deletionProtection: false,
  zitadelImage: 'ghcr.io/zitadel/zitadel:v4.11.0',
};
