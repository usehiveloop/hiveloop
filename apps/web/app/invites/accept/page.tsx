"use client"

import * as React from "react"
import { Suspense, useCallback, useMemo, useState } from "react"
import Link from "next/link"
import { useRouter, useSearchParams } from "next/navigation"
import { Button, buttonVariants } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Logo } from "@/components/logo"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon } from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { localPart } from "@/lib/email"
import { toast } from "sonner"

function CenterCard({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex items-center justify-center px-4 py-10 bg-background">
      <div className="w-full max-w-md">
        <div className="flex justify-center mb-6">
          <Logo />
        </div>
        <div className="rounded-2xl border border-border bg-card p-6 shadow-sm">{children}</div>
      </div>
    </div>
  )
}

function AcceptInviteContents() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const token = searchParams.get("token") ?? ""

  // Invite preview (public)
  const previewQuery = $api.useQuery(
    "get",
    "/v1/invites/{token}",
    { params: { path: { token } } },
    { enabled: token.length > 0, retry: false },
  )

  // Current auth status — no retry, errors are treated as "logged out".
  const meQuery = $api.useQuery("get", "/auth/me", {}, { retry: false })
  const me = meQuery.data

  const acceptMutation = $api.useMutation("post", "/v1/invites/{token}/accept")
  const declineMutation = $api.useMutation("post", "/v1/invites/{token}/decline")
  const logoutMutation = $api.useMutation("post", "/auth/logout")

  const [accepted, setAccepted] = useState<{ orgName: string } | null>(null)
  const [declined, setDeclined] = useState(false)

  const nextHref = useMemo(() => {
    if (!token) return "/auth"
    return `/auth`
  }, [token])

  const handleAccept = useCallback(() => {
    acceptMutation.mutate(
      { params: { path: { token } } },
      {
        onSuccess: (resp) => {
          setAccepted({ orgName: resp?.org_name ?? "" })
          setTimeout(() => router.replace("/w"), 1500)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to accept invitation"))
        },
      },
    )
  }, [acceptMutation, token, router])

  const handleDecline = useCallback(() => {
    declineMutation.mutate(
      { params: { path: { token } } },
      {
        onSuccess: () => {
          setDeclined(true)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to decline invitation"))
        },
      },
    )
  }, [declineMutation, token])

  const handleLogoutAndGoToAuth = useCallback(() => {
    logoutMutation.mutate(
      { body: {} },
      {
        onSettled: () => {
          router.replace("/auth")
        },
      },
    )
  }, [logoutMutation, router])

  // --- State rendering ---

  if (!token) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">Invalid link</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          This invitation link is missing its token.
        </p>
        <div className="mt-6">
          <Link href="/auth" className={buttonVariants({ className: "w-full" })}>
            Go to sign in
          </Link>
        </div>
      </CenterCard>
    )
  }

  if (previewQuery.isLoading || meQuery.isLoading) {
    return (
      <CenterCard>
        <div className="flex flex-col items-center py-10">
          <HugeiconsIcon icon={Loading03Icon} size={24} className="animate-spin text-muted-foreground" />
          <p className="mt-3 text-sm text-muted-foreground">Loading invitation…</p>
        </div>
      </CenterCard>
    )
  }

  if (previewQuery.isError || !previewQuery.data) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">Invitation unavailable</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          This invitation is no longer valid. It may have expired, been revoked, or already been used.
        </p>
        <div className="mt-6">
          <Link href="/auth" className={buttonVariants({ className: "w-full" })}>
            Back to sign in
          </Link>
        </div>
      </CenterCard>
    )
  }

  const invite = previewQuery.data
  const inviteEmail = invite.email ?? ""
  const orgName = invite.org_name ?? "this workspace"

  if (declined) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">Invitation declined</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          You&apos;ve declined the invitation to {orgName}.
        </p>
        <div className="mt-6">
          <Link href="/w" className={buttonVariants({ className: "w-full" })}>
            Go to workspace
          </Link>
        </div>
      </CenterCard>
    )
  }

  if (accepted) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">Welcome to {accepted.orgName || orgName}</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          You&apos;re now a member. Redirecting you to the workspace…
        </p>
      </CenterCard>
    )
  }

  // Not logged in
  if (meQuery.isError || !me?.user) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">
          {invite.inviter_name ?? "Someone"} invited you to {orgName}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Sent to <span className="font-medium text-foreground">{inviteEmail}</span> — role{" "}
          <Badge variant="secondary" className="text-2xs">{invite.role}</Badge>
        </p>
        <div className="mt-6 flex flex-col gap-2">
          <Link href={nextHref} className={buttonVariants({ className: "w-full" })}>
            Sign in to accept
          </Link>
          <Link href={nextHref} className={buttonVariants({ variant: "outline", className: "w-full" })}>
            Create account
          </Link>
        </div>
        <p className="mt-4 text-mini text-muted-foreground text-center">
          After signing in, come back to this link to accept the invitation.
        </p>
      </CenterCard>
    )
  }

  // Logged in, check email match
  const userEmail = (me.user.email ?? "").toLowerCase().trim()
  const expected = inviteEmail.toLowerCase().trim()

  if (userEmail !== expected) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">Wrong account</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          This invite was sent to <span className="font-medium text-foreground">{inviteEmail}</span>.
          You&apos;re signed in as <span className="font-medium text-foreground">{me.user.email}</span>.
        </p>
        <div className="mt-6">
          <Button
            className="w-full"
            onClick={handleLogoutAndGoToAuth}
            loading={logoutMutation.isPending}
          >
            Sign out and sign in as {localPart(inviteEmail)}
          </Button>
        </div>
      </CenterCard>
    )
  }

  // Logged in, matching email
  return (
    <CenterCard>
      <h1 className="text-lg font-semibold text-foreground">
        Join {orgName}
      </h1>
      <p className="mt-2 text-sm text-muted-foreground">
        {invite.inviter_name ?? "An admin"} invited you to {orgName} as{" "}
        <Badge variant="secondary" className="text-2xs">{invite.role}</Badge>
      </p>
      <div className="mt-6 flex flex-col gap-2">
        <Button
          className="w-full"
          onClick={handleAccept}
          loading={acceptMutation.isPending}
          disabled={acceptMutation.isPending || declineMutation.isPending}
        >
          Join {orgName}
        </Button>
        <Button
          variant="outline"
          className="w-full"
          onClick={handleDecline}
          loading={declineMutation.isPending}
          disabled={acceptMutation.isPending || declineMutation.isPending}
        >
          Decline
        </Button>
      </div>
    </CenterCard>
  )
}

export default function AcceptInvitePage() {
  return (
    <Suspense
      fallback={
        <CenterCard>
          <div className="flex justify-center py-10">
            <HugeiconsIcon icon={Loading03Icon} size={24} className="animate-spin text-muted-foreground" />
          </div>
        </CenterCard>
      }
    >
      <AcceptInviteContents />
    </Suspense>
  )
}
