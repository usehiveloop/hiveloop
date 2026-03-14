import Link from "next/link";
import { getLogtoContext } from "@logto/next/server-actions";
import { getLogtoConfig } from "@/lib/logto";
import { Button } from "@/components/ui/button";
import { LockIcon } from "@/components/icons";

async function getAuthState() {
  try {
    const { isAuthenticated } = await getLogtoContext(getLogtoConfig());
    return isAuthenticated;
  } catch {
    return false;
  }
}

export async function Nav() {
  const isAuthenticated = await getAuthState();

  return (
    <nav className="flex h-16 shrink-0 items-center justify-center border-b border-border">
      <div className="flex h-full w-full max-w-7xl items-center justify-between px-20">
      <Link href="/" className="flex items-center gap-2.5">
        <LockIcon />
        <span className="font-bricolage text-lg font-semibold tracking-tight text-foreground">
          llmvault
        </span>
      </Link>

      <div className="flex h-full items-center gap-8">
        {/* Features dropdown */}
        <div className="group/features relative flex h-full items-center">
          <button className="flex items-center gap-1.5">
            <span className="text-sm text-muted-foreground">Features</span>
            <svg width="10" height="6" viewBox="0 0 10 6" fill="none">
              <path d="M1 1L5 5L9 1" stroke="#A1A1AB" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
          <div className="absolute top-full -left-4 z-50 hidden pt-0 group-hover/features:block">
            <div className="flex w-80 flex-col gap-0.5 border border-border bg-surface p-2">
              <Link href="/features/connect-ui" className="flex items-start gap-3.5 px-4 py-3.5">
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#8B5CF6" strokeWidth="1.5" className="mt-px shrink-0">
                  <path d="M13.5 16.875h3.375m0 0h3.375m-3.375 0V13.5m0 3.375v3.375M6 10.5h2.25a2.25 2.25 0 0 0 2.25-2.25V6a2.25 2.25 0 0 0-2.25-2.25H6A2.25 2.25 0 0 0 3.75 6v2.25A2.25 2.25 0 0 0 6 10.5Zm0 9.75h2.25A2.25 2.25 0 0 0 10.5 18v-2.25a2.25 2.25 0 0 0-2.25-2.25H6a2.25 2.25 0 0 0-2.25 2.25V18A2.25 2.25 0 0 0 6 20.25Zm9.75-9.75H18a2.25 2.25 0 0 0 2.25-2.25V6A2.25 2.25 0 0 0 18 3.75h-2.25A2.25 2.25 0 0 0 13.5 6v2.25a2.25 2.25 0 0 0 2.25 2.25Z" />
                </svg>
                <div className="flex flex-col gap-1">
                  <span className="text-sm font-medium leading-4.5 text-[#E4E1EC]">Connect UI</span>
                  <span className="text-xs leading-[17px] text-[#9794A3]">Drop-in BYOK onboarding modal</span>
                </div>
              </Link>
              <Link href="/features/encryption" className="flex items-start gap-3.5 px-4 py-3.5">
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#8B5CF6" strokeWidth="1.5" className="mt-px shrink-0">
                  <path d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 0 0 2.25-2.25v-6.75a2.25 2.25 0 0 0-2.25-2.25H6.75a2.25 2.25 0 0 0-2.25 2.25v6.75a2.25 2.25 0 0 0 2.25 2.25Z" />
                </svg>
                <div className="flex flex-col gap-1">
                  <span className="text-sm font-medium leading-4.5 text-[#E4E1EC]">Envelope Encryption</span>
                  <span className="text-xs leading-[17px] text-[#9794A3]">AES-256-GCM with Vault Transit</span>
                </div>
              </Link>
            </div>
          </div>
        </div>

        <Link href="/docs" className="text-sm text-muted-foreground">Docs</Link>
        <Link href="/pricing" className="text-sm text-muted-foreground">Pricing</Link>
        <Link href="/architecture" className="text-sm text-muted-foreground">Architecture</Link>
        <a href="https://github.com/useportal/llmvault" target="_blank" rel="noopener noreferrer" className="text-sm text-muted-foreground">GitHub</a>
        {isAuthenticated ? (
          <Button render={<Link href="/dashboard" />} className="h-auto px-5 py-2 text-sm font-medium">
            Dashboard
          </Button>
        ) : (
          <Button render={<Link href="/sign-in" />} className="h-auto px-5 py-2 text-sm font-medium">
            Sign In
          </Button>
        )}
      </div>
      </div>
    </nav>
  );
}
