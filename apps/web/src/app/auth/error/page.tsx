"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";
import Link from "next/link";
import { LockIcon } from "@/components/icons";

const ERROR_MESSAGES: Record<string, string> = {
  Configuration: "There is a problem with the server configuration.",
  AccessDenied: "Access denied. You do not have permission to sign in.",
  Verification: "The verification link has expired or has already been used.",
  OAuthSignin: "Could not start the sign-in flow.",
  OAuthCallback: "Authentication failed during the callback.",
  OAuthCreateAccount: "Could not create your account.",
  EmailCreateAccount: "Could not create your account.",
  Callback: "Authentication callback failed.",
  OAuthAccountNotLinked: "This email is already linked to another account.",
  SessionRequired: "Please sign in to access this page.",
  Default: "An unexpected error occurred.",
};

function ErrorContent() {
  const searchParams = useSearchParams();
  const error = searchParams.get("error") ?? "Default";
  const message = ERROR_MESSAGES[error] ?? ERROR_MESSAGES.Default;

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="flex w-full max-w-sm flex-col items-center gap-8 px-6">
        <div className="flex items-center gap-2.5">
          <LockIcon />
          <span
            className="text-xl font-semibold tracking-tight text-foreground"
            style={{ fontFamily: "var(--font-bricolage)" }}
          >
            llmvault
          </span>
        </div>

        <div className="flex w-full flex-col gap-6 border border-border bg-card p-8">
          <div className="flex flex-col gap-2 text-center">
            <h1 className="text-lg font-semibold text-foreground">
              Authentication Error
            </h1>
            <p className="text-sm text-muted-foreground">{message}</p>
          </div>

          <Link
            href="/auth/login"
            className="flex w-full items-center justify-center bg-primary px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary/90"
          >
            Try again
          </Link>
        </div>

        <Link href="/" className="text-xs text-muted-foreground hover:text-foreground">
          Back to home
        </Link>
      </div>
    </div>
  );
}

export default function AuthErrorPage() {
  return (
    <Suspense>
      <ErrorContent />
    </Suspense>
  );
}
