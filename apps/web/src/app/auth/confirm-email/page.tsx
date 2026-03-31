"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { LockIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { $api } from "@/api/client";

export default function ConfirmEmailPage() {
  return (
    <Suspense>
      <ConfirmEmailContent />
    </Suspense>
  );
}

function ConfirmEmailContent() {
  const searchParams = useSearchParams();
  const email = searchParams.get("email") ?? "";
  const [sent, setSent] = useState(false);

  const resendMutation = $api.useMutation("post", "/auth/resend-confirmation", {
    onSuccess: () => setSent(true),
  });

  function handleResend() {
    setSent(false);
    resendMutation.mutate({ body: { email } });
  }

  const error = resendMutation.error
    ? String(resendMutation.error).includes("429")
      ? "Please wait before requesting another email."
      : resendMutation.error instanceof Error
        ? resendMutation.error.message
        : "Failed to resend. Please try again."
    : null;

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="flex w-full max-w-lg flex-col items-center gap-8 px-6">
        <div className="flex items-center gap-2.5">
          <LockIcon />
          <span
            className="text-xl font-semibold tracking-tight text-foreground"
            style={{ fontFamily: "var(--font-bricolage)" }}
          >
            llmvault
          </span>
        </div>

        <div className="flex w-full flex-col gap-8 border border-border bg-card px-8 py-16">
          <div className="flex flex-col gap-2 text-center">
            <h1 className="text-lg font-semibold text-foreground mb-4">Check your email</h1>
            <p className="text-sm text-muted-foreground">
              We sent a confirmation link to{" "}
              {email ? (
                <span className="font-medium text-foreground">{email}</span>
              ) : (
                "your email"
              )}
              . Click the link to verify your account and access the dashboard.
            </p>
          </div>

          {error && (
            <div className="border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {error}
            </div>
          )}

          {sent && (
            <div className="border border-primary/30 bg-primary/10 px-4 py-3 text-sm text-primary">
              Confirmation email sent. Check your inbox.
            </div>
          )}

          <div className="flex justify-center">
            <Button onClick={handleResend} loading={resendMutation.isPending} size='lg'>
            Resend confirmation email
          </Button>
          </div>

          <p className="text-center text-sm text-muted-foreground">
            Wrong email?{" "}
            <Link href="/auth/register" className="text-primary hover:underline">
              Register again
            </Link>
          </p>
        </div>

        <Link href="/auth/login" className="text-xs text-muted-foreground hover:text-foreground">
          Back to login
        </Link>
      </div>
    </div>
  );
}
