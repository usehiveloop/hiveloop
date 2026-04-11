import { defineCollections, defineDocs, defineConfig } from 'fumadocs-mdx/config';
import { z } from 'zod';

export const docs = defineDocs({
  dir: 'content/docs',
});

// Blog posts. Each .mdx file under content/blog/ has frontmatter validated
// against this schema at build time. The slug is derived from the file
// name. lib/source.tsx wraps the raw collection with typed helpers so the
// page files can stay clean.
export const posts = defineCollections({
  type: 'doc',
  dir: 'content/blog',
  schema: z.object({
    title: z.string(),
    description: z.string(),
    date: z.string(),
    author: z.string(),
    authorAvatar: z.string(),
    category: z.string(),
    readTime: z.string(),
  }),
});

export default defineConfig();
