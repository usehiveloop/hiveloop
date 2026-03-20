import { readFile } from "fs/promises";
import { join } from "path";
import matter from "gray-matter";
import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import rehypeStringify from "rehype-stringify";

export type DocFrontmatter = {
  title: string;
  description?: string;
};

export type DocContent = {
  frontmatter: DocFrontmatter;
  content: string;
};

export async function getDocContent(slug: string[]): Promise<DocContent | null> {
  try {
    const filePath = join(
      process.cwd(),
      "src/content/docs",
      ...slug
    ) + ".md";
    
    const fileContent = await readFile(filePath, "utf-8");
    const { data, content } = matter(fileContent);
    
    // Process markdown to HTML
    const processed = await unified()
      .use(remarkParse)
      .use(remarkGfm)
      .use(rehypeHighlight)
      .use(rehypeStringify)
      .process(content);
    
    return {
      frontmatter: {
        title: data.title || "Untitled",
        description: data.description,
      },
      content: String(processed),
    };
  } catch {
    return null;
  }
}

export function extractToc(content: string): { id: string; label: string; depth: number }[] {
  const headings: { id: string; label: string; depth: number }[] = [];
  const lines = content.split("\n");
  
  for (const line of lines) {
    const match = line.match(/^(#{2,3})\s+(.+)$/);
    if (match) {
      const depth = match[1].length;
      const label = match[2].trim();
      const id = label
        .toLowerCase()
        .replace(/[^\w\s-]/g, "")
        .replace(/\s+/g, "-");
      
      headings.push({ id, label, depth });
    }
  }
  
  return headings;
}
