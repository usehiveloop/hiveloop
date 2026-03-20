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
        className="prose prose-invert prose-zinc max-w-none
          prose-headings:font-mono prose-headings:text-[#E4E1EC]
          prose-h2:text-[22px] prose-h2:font-medium prose-h2:leading-7 prose-h2:mt-10 prose-h2:mb-4
          prose-h3:text-lg prose-h3:font-medium prose-h3:leading-6 prose-h3:mt-8 prose-h3:mb-3
          prose-p:text-[15px] prose-p:leading-6.5 prose-p:text-[#9794A3]
          prose-p:my-4
          prose-a:text-[#A78BFA] prose-a:no-underline hover:prose-a:underline
          prose-code:text-[#A78BFA] prose-code:bg-[#1A1A1F] prose-code:px-1.5 prose-code:py-0.5 prose-code:rounded prose-code:text-[13px]
          prose-pre:bg-[#0D0D0F] prose-pre:border prose-pre:border-[#27262B] prose-pre:rounded-lg
          prose-pre:p-4 prose-pre:my-6
          prose-ol:my-4 prose-ol:pl-6
          prose-ul:my-4 prose-ul:pl-6
          prose-li:text-[15px] prose-li:leading-6.5 prose-li:text-[#9794A3] prose-li:my-1.5
          prose-table:my-6 prose-table:w-full prose-table:text-left
          prose-thead:border-b prose-thead:border-[#27262B]
          prose-th:text-[13px] prose-th:font-semibold prose-th:text-[#E4E1EC] prose-th:py-3 prose-th:px-4
          prose-td:text-[14px] prose-td:text-[#9794A3] prose-td:py-3 prose-td:px-4
          prose-tbody:divide-y prose-tbody:divide-[#27262B]
          prose-hr:border-[#27262B] prose-hr:my-8
          prose-strong:text-[#E4E1EC] prose-strong:font-semibold
          prose-blockquote:border-l-2 prose-blockquote:border-[#A78BFA] prose-blockquote:pl-4 prose-blockquote:my-6
          prose-blockquote:text-[#9794A3]
          [&_.hljs]:bg-transparent [&_.hljs]:p-0
        "
        dangerouslySetInnerHTML={{ __html: content }}
      />
    </>
  );
}
