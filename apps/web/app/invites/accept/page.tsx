"use client"

import * as React from "react"
import { Suspense } from "react"
import Link from "next/link"
import { useRouter, useSearchParams } from "next/navigation"
import { Button, buttonVariants } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Logo } from "@/components/logo"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon, Mail01Icon, AlertCircleIcon, Tick02Icon, Cancel01Icon } from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { localPart } from "@/lib/email"
import { toast } from "sonner"

// ── Accept invite token persistence ─────────────────────────────────────

function storeInviteToken(token: string) {
  try { sessionStorage.setItem("pending_invite_token", token) } catch {}
}

function retrieveInviteToken(): string | null {
  try { return sessionStorage.getItem("pending_invite_token") } catch { return null }
}

function clearInviteToken() {
  try { sessionStorage.removeItem("pending_invite_token") } catch {}
}

// ── Welcome toast for invite-based join ─────────────────────────────────

function showWelcomeToast(orgName: string) {
  toast.success(
    <span>
      Welcome to <strong>{orgName}</strong>! You're now a member.
    </span>,
    { duration: 5000 }
  )
}

// ── Center card wrapper ───────────────────────────────────────────────

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

// ── Main content ──────────────────────────────────────────────────────

function AcceptInviteContents() {
  const searchParams = useSearchParams()
  const router = useRouter()

  // Get token from URL or sessionStorage (survives auth redirect)
  const urlToken = searchParams.get("token") ?? ""
  const [token, setToken] = React.useState(() => urlToken || retrieveInviteToken() || "")

  // Store token when it appears in URL (for auth redirect survival)
  React.useEffect(() => {
    if (urlToken && urlToken !== token) {
      setToken(urlToken)
      storeInviteToken(urlToken)
    }
  }, [urlToken, token])

  // Invite preview (public)
  const previewQuery = $api.useQuery(
    "get",
    "/v1/invites/{token}",
    { params: { path: { token } } },
    { enabled: token.length > 0, retry: false }
  )

  // Current auth status
  const meQuery = $api.useQuery("get", "/auth/me", {}, { retry: false })
  const me = meQuery.data
  const isLoggedIn = !!me?.user

  const acceptMutation = $api.useMutation("post", "/v1/invites/{token}/accept")
  const declineMutation = $api.useMutation("post", "/v1/invites/{token}/decline")
  const logoutMutation = $api.useMutation("post", "/auth/logout")

  const [accepted, setAccepted] = React.useState<{ orgName: string } | null>(null)
  const [declined, setDeclined] = React.useState(false)

  // Auto-accept after returning from auth flow
  const hasAttemptedAutoAccept = React.useRef(false)
  React.useEffect(() => {
    if (
      !hasAttemptedAutoAccept.current &&
      token &&
      previewQuery.data &&
      isLoggedIn &&
      me?.user?.email?.toLowerCase() === (previewQuery.data.email ?? "").toLowerCase() &&
      !accepted &&
      !acceptMutation.isPending
    ) {
      hasAttemptedAutoAccept.current = true
      // Small delay to let the page settle
      const timer = setTimeout(() => {
        handleAccept()
      }, 500)
      return () => clearTimeout(timer)
    }
  }, [token, previewQuery.data, isLoggedIn, me?.user?.email])

  const handleAccept = React.useCallback(() => {
    acceptMutation.mutate(
      { params: { path: { token } } },
      {
        onSuccess: (resp) => {
          clearInviteToken()
          setAccepted({ orgName: resp?.org_name ?? "" })
          showWelcomeToast(resp?.org_name ?? "")
          setTimeout(() => router.replace("/w"), 2000)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to accept invitation"))
        },
      },
    )
  }, [acceptMutation, token, router])

  const handleDecline = React.useCallback(() => {
    declineMutation.mutate(
      { params: { path: { token } } },
      {
        onSuccess: () => {
          clearInviteToken()
          setDeclined(true)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to decline invitation"))
        },
      },
    )
  }, [declineMutation, token])

  const handleLogoutAndGoToAuth = React.useCallback(() => {
    logoutMutation.mutate(
      { body: {} },
      {
        onSettled: () => {
          // Keep token in sessionStorage so we can auto-accept after login
          router.replace("/auth")
        },
      },
    )
  }, [logoutMutation, router])

  // ── State rendering ─────────────────────────────────────────────

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

  if (previewQuery.isLoading || (meQuery.isLoading && token)) {
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
        <div className="flex justify-center mb-4">
          <div className="flex size-12 items-center justify-center rounded-full bg-destructive/10">
            <HugeiconsIcon icon={Cancel01Icon} size={24} className="text-destructive" />
          </div>
        </div>
        <h1 className="text-lg font-semibold text-foreground text-center">Invitation no longer available</h1>
        <p className="mt-2 text-sm text-muted-foreground text-center">
          This invitation is no longer valid. It may have expired, been revoked, or already been used.
        </p>
        <div className="mt-6 flex flex-col gap-2">
          <Link href="/auth" className={buttonVariants({ className: "w-full" })}>
            Sign in
          </Link>
          <a
            href="mailto:support@hiveloop.com"
            className={buttonVariants({ variant: "outline", className: "w-full" })}
          >
            Contact support
          </a>
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
        <div className="flex justify-center mb-4">
          <div className="flex size-12 items-center justify-center rounded-full bg-muted">
            <HugeiconsIcon icon={Tick02Icon} size={24} className="text-muted-foreground" />
          </div>
        </div>
        <h1 className="text-lg font-semibold text-foreground text-center">Invitation declined</h1>
        <p className="mt-2 text-sm text-muted-foreground text-center">
          You've declined the invitation to {orgName}.
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
        <div className="flex justify-center mb-4">
          <div className="flex size-12 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
            <HugeiconsIcon icon={Tick02Icon} size={24} className="text-green-600 dark:text-green-400" />
          </div>
        </div>
        <h1 className="text-lg font-semibold text-foreground text-center">Welcome to {accepted.orgName || orgName}</h1>
        <p className="mt-2 text-sm text-muted-foreground text-center">
          You're now a member. Redirecting you to the workspace…
        </p>
      </CenterCard>
    )
  }

  // Not logged in
  if (meQuery.isError || !me?.user) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground text-center">
          {invite.inviter_name ?? "Someone"} invited you to {orgName}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground text-center">
          Sent to <span className="font-medium text-foreground">{inviteEmail}</span> — role{" "}
          <Badge variant="secondary" className="text-[10px]">{invite.role}</Badge>
        </p>
        <div className="mt-6 flex flex-col gap-2">
          <Link
            href={`/auth?redirect=${encodeURIComponent(`/invites/accept?token=${token}`)}`}
            className={buttonVariants({ className: "w-full" })}
          >
            Sign in to accept
          </Link>
          <Link
            href={`/auth?redirect=${encodeURIComponent(`/invites/accept?token=${token}`)}`}
            className={buttonVariants({ variant: "outline", className: "w-full" })}
          >
            Create account
          </Link>
        </div>
        <p className="mt-4 text-[11px] text-muted-foreground text-center">
          After signing in, you'll be automatically added to {orgName}.
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
        <div className="flex justify-center mb-4">
          <div className="flex size-12 items-center justify-center rounded-full bg-amber-100 dark:bg-amber-900/30">
            <HugeiconsIcon icon={AlertCircleIcon} size={24} className="text-amber-600 dark:text-amber-400" />
          </div>
        </div>
        <h1 className="text-lg font-semibold text-foreground text-center">Wrong account</h1>
        <p className="mt-2 text-sm text-muted-foreground text-center">
          This invite was sent to <span className="font-medium text-foreground">{inviteEmail}</span>.
          You're signed in as <span className="font-medium text-foreground">{me.user.email}</span>.
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

  // Logged in, matching email — show accept/decline
  return (
    <CenterCard>
      <h1 className="text-lg font-semibold text-foreground text-center">
        Join {orgName}
      </h1>
      <p className="mt-2 text-sm text-muted-foreground text-center">
        {invite.inviter_name ?? "An admin"} invited you to {orgName} as{" "}
        <Badge variant="secondary" className="text-[10px]">{invite.role}</Badge>
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
