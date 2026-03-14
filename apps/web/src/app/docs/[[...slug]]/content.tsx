"use client";

import { DocsToc, type TocItem } from "@/components/docs-toc";

export function DocsContent({
  slug,
  toc,
}: {
  slug: string;
  toc: TocItem[];
}) {
  return (
    <>
      <DocsToc items={toc} />
      <div className="flex flex-col gap-8">
        {toc.map((item) => (
          <section key={item.id} id={item.id} className="flex flex-col gap-3">
            <h2 className="font-mono text-[22px] font-medium leading-7 text-[#E4E1EC]">
              {item.label}
            </h2>
            <p className="text-[15px] leading-6.5 text-[#9794A3]">
              Content for {item.label} will go here. This is a placeholder for the{" "}
              <span className="font-mono text-[13px] text-[#A78BFA]">{slug}</span>{" "}
              documentation page.
            </p>
          </section>
        ))}
      </div>
    </>
  );
}
