import { docs, posts as rawPosts } from 'collections/server';
import { loader } from 'fumadocs-core/source';
import { HugeiconsIcon } from '@hugeicons/react';
import { BookOpen01Icon, CompassIcon, DashboardSquare01Icon, ApiIcon } from '@hugeicons/core-free-icons';
import type { MDXContent } from 'mdx/types';

const icons: Record<string, typeof BookOpen01Icon> = {
  BookOpen01Icon,
  CompassIcon,
  DashboardSquare01Icon,
  ApiIcon,
};

export const source = loader({
  baseUrl: '/docs',
  source: docs.toFumadocsSource(),
  icon(name) {
    if (!name || !(name in icons)) return undefined;
    return <HugeiconsIcon icon={icons[name]} className="size-full" />;
  },
});

// Blog posts come from a flat fumadocs-mdx collection. Each .mdx file in
// content/blog/ becomes one entry. We wrap the raw collection here with a
// strongly-typed helper layer so the page files don't have to handle the
// schema-inference quirks of fumadocs's StandardSchemaV1 bridge.
//
// The slug is derived from the .mdx filename (basename without extension).
// Adding a new post is as simple as dropping a file in content/blog/.

export interface PostFrontmatter {
  title: string;
  description: string;
  date: string;
  author: string;
  authorAvatar: string;
  category: string;
  readTime: string;
}

export interface Post {
  slug: string;
  data: PostFrontmatter;
  body: MDXContent;
}

// The fumadocs entry shape we care about. The actual entry is wider — it
// includes structuredData, toc, _exports, etc. — but the blog only needs
// the frontmatter and the MDX body.
interface RawPostEntry extends PostFrontmatter {
  body: MDXContent;
  info: { path: string; fullPath: string };
}

function deriveSlugFromPath(path: string): string {
  // Strip directory prefix and .mdx extension. The fumadocs `info.path` is
  // typically already a clean filename like "my-post.mdx" but we handle
  // both shapes defensively.
  const filename = path.split('/').pop() ?? path;
  return filename.replace(/\.mdx$/i, '');
}

function toPost(entry: RawPostEntry): Post {
  return {
    slug: deriveSlugFromPath(entry.info.path),
    data: {
      title: entry.title,
      description: entry.description,
      date: entry.date,
      author: entry.author,
      authorAvatar: entry.authorAvatar,
      category: entry.category,
      readTime: entry.readTime,
    },
    body: entry.body,
  };
}

/**
 * Returns every blog post sorted by date descending (newest first).
 * Used by the blog index page.
 */
export function getAllPosts(): Post[] {
  const entries = rawPosts as unknown as RawPostEntry[];
  return entries
    .map(toPost)
    .sort((first, second) => new Date(second.data.date).getTime() - new Date(first.data.date).getTime());
}

/**
 * Returns one post by slug, or undefined if no post matches. Used by the
 * dynamic [slug] route.
 */
export function findPostBySlug(slug: string): Post | undefined {
  const entries = rawPosts as unknown as RawPostEntry[];
  for (const entry of entries) {
    if (deriveSlugFromPath(entry.info.path) === slug) {
      return toPost(entry);
    }
  }
  return undefined;
}
