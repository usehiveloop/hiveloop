"use client";

import { useEffect, useState, useSyncExternalStore } from "react";
import { createPortal } from "react-dom";

function subscribeNoop() {
  return () => {};
}
function getDocsTocElement() {
  return document.getElementById("docs-toc");
}
function getDocsTocServer() {
  return null;
}

export type TocItem = { id: string; label: string; depth?: number };

export function DocsToc({ items }: { items: TocItem[] }) {
  const [activeId, setActiveId] = useState<string>("");
  const portalTarget = useSyncExternalStore(subscribeNoop, getDocsTocElement, getDocsTocServer);

  useEffect(() => {
    if (items.length === 0) return;

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setActiveId(entry.target.id);
          }
        }
      },
      { rootMargin: "-80px 0px -60% 0px", threshold: 0 }
    );

    for (const item of items) {
      const el = document.getElementById(item.id);
      if (el) observer.observe(el);
    }

    return () => observer.disconnect();
  }, [items]);

  if (!portalTarget || items.length === 0) return null;

  return createPortal(
    <div className="flex flex-col gap-4">
      <span className="text-[11px] font-semibold uppercase leading-3.5 tracking-wider text-[#9794A3]">
        On this page
      </span>
      <div className="flex flex-col gap-2.5 border-l border-border pl-3">
        {items.map((item) => {
          const isActive = activeId === item.id;
          return (
            <a
              key={item.id}
              href={`#${item.id}`}
              className={`text-[13px] leading-4 transition-colors ${
                isActive
                  ? "-ml-[13px] border-l border-primary pl-[11px] text-[#A78BFA]"
                  : "text-[#9794A3] hover:text-[#E4E1EC]"
              }`}
              style={item.depth && item.depth > 0 ? { paddingLeft: `${item.depth * 12}px` } : undefined}
            >
              {item.label}
            </a>
          );
        })}
      </div>
    </div>,
    portalTarget
  );
}
