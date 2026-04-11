import { createOpenAPI } from 'fumadocs-openapi/server';

// Relative path resolved at build time from apps/web cwd. Must match the
// `document` prop in every generated apps/web/content/docs/**.mdx file so
// fumadocs-openapi picks the right spec regardless of where CI checks out.
export const openapi = createOpenAPI({
  input: ['lib/openapi-spec.json'],
});
