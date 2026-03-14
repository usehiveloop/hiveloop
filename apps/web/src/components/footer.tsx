import Link from "next/link";
import { LockIcon } from "@/components/icons";

export function Footer() {
  return (
    <footer className="flex items-center justify-center border-t border-border">
      <div className="flex w-full max-w-7xl items-center justify-between px-20 py-8">
        <Link href="/" className="flex items-center gap-2.5">
          <LockIcon size={20} />
          <span className="font-bricolage text-sm font-semibold leading-4.5 tracking-tight text-dim">
            llmvault
          </span>
        </Link>
        <div className="flex items-center gap-6">
          <Link href="/docs" className="text-[13px] leading-4 text-dim">Docs</Link>
          <Link href="/pricing" className="text-[13px] leading-4 text-dim">Pricing</Link>
          <Link href="/architecture" className="text-[13px] leading-4 text-dim">Architecture</Link>
          <a href="https://github.com/llmvault/llmvault" target="_blank" rel="noopener noreferrer" className="text-[13px] leading-4 text-dim">GitHub</a>
          <a href="https://twitter.com/llmvault" target="_blank" rel="noopener noreferrer" className="text-[13px] leading-4 text-dim">Twitter</a>
        </div>
        <span className="text-[13px] leading-4 text-dim">
          &copy; 2026 LLMVault. All rights reserved.
        </span>
      </div>
    </footer>
  );
}
