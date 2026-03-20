"use client";

import { DocsToc } from "@/components/docs-toc";
import "highlight.js/styles/github-dark.css";

export type TocItem = { id: string; label: string; depth?: number };

export function DocsContent({
  content,
  toc,
}: {
  content: string;
  toc: TocItem[];
}) {
  return (
    <>
      <DocsToc items={toc} />
      <div
        className="prose prose-invert max-w-none docs-prose"
        dangerouslySetInnerHTML={{ __html: content }}
      />
    </>
  );
}
