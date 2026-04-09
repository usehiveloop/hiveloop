import { generateFiles } from 'fumadocs-openapi';
import { createOpenAPI } from 'fumadocs-openapi/server';
import fs from 'node:fs';
import path from 'node:path';

// Read the spec and inject top-level tags from operation tags
const specPath = path.resolve(process.cwd(), '../../docs/openapi.json');
const spec = JSON.parse(fs.readFileSync(specPath, 'utf-8'));

const tagNames = new Set<string>();
for (const methods of Object.values(spec.paths ?? {})) {
  for (const details of Object.values(methods as Record<string, { tags?: string[] }>)) {
    if (details?.tags) {
      for (const tag of details.tags) {
        if (tag !== 'admin') tagNames.add(tag);
      }
    }
  }
}

const tagDisplayNames: Record<string, string> = {
  agents: 'Agents',
  'api-keys': 'API Keys',
  audit: 'Audit',
  auth: 'Authentication',
  billing: 'Billing',
  'connect-sessions': 'Connect Sessions',
  connections: 'Connections',
  conversations: 'Conversations',
  credentials: 'Credentials',
  'custom-domains': 'Custom Domains',
  forge: 'Forge',
  generations: 'Generations',
  identities: 'Identities',
  'in-connections': 'Internal Connections',
  'in-integrations': 'Internal Integrations',
  integrations: 'Integrations',
  marketplace: 'Marketplace',
  oauth: 'OAuth',
  orgs: 'Organizations',
  providers: 'Providers',
  reporting: 'Reporting',
  'sandbox-templates': 'Sandbox Templates',
  sandboxes: 'Sandboxes',
  settings: 'Settings',
  tokens: 'Tokens',
  usage: 'Usage',
  widget: 'Widget',
};

spec.tags = [...tagNames].map((name) => ({
  name,
  description: tagDisplayNames[name] ?? name,
}));

// Filter out admin paths
for (const [pathKey, methods] of Object.entries(spec.paths ?? {})) {
  if (pathKey.startsWith('/admin/')) {
    delete spec.paths[pathKey];
    continue;
  }
  for (const [method, details] of Object.entries(methods as Record<string, { tags?: string[] }>)) {
    if (details?.tags?.includes('admin')) {
      delete (methods as Record<string, unknown>)[method];
    }
  }
}

// Write cleaned spec to a permanent location (referenced at runtime by APIPage)
const cleanedSpecPath = path.resolve(process.cwd(), 'lib/openapi-spec.json');
fs.writeFileSync(cleanedSpecPath, JSON.stringify(spec, null, 2));

const openapi = createOpenAPI({
  input: [cleanedSpecPath],
});

await generateFiles({
  input: openapi,
  output: './content/docs/api-reference',
  groupBy: 'tag',
  meta: {
    groupStyle: 'folder',
  },
});

// Restore root meta fields that the generator overwrites
const metaPath = path.resolve(process.cwd(), 'content/docs/api-reference/meta.json');
const meta = JSON.parse(fs.readFileSync(metaPath, 'utf-8'));
if (!meta.title) {
  meta.title = 'API Reference';
  meta.icon = 'ApiIcon';
  meta.root = true;
  if (!meta.pages.includes('index')) {
    meta.pages.unshift('index');
  }
  fs.writeFileSync(metaPath, JSON.stringify(meta, null, 2));
}

console.log('OpenAPI docs generated.');
