import { generateFiles } from 'fumadocs-openapi';
import { createOpenAPI } from 'fumadocs-openapi/server';
import path from 'node:path';

const openapi = createOpenAPI({
  input: [path.resolve(process.cwd(), '../../docs/openapi.json')],
});

await generateFiles({
  input: openapi,
  output: './content/docs/api-reference',
  meta: true,
});

console.log('OpenAPI docs generated.');
