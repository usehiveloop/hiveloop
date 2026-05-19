"use client"

import * as React from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { ImagePicker } from "@/components/image-picker"
import { useAuth } from "@/lib/auth/auth-context"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

export function ProfileSettings() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const meQuery = $api.useQuery("get", "/auth/me", {}, { retry: false })
  const meData = meQuery.data as Record<string, unknown> | undefined
  const oauthProviders = (meData?.oauth_providers as string[]) ?? []

  const savedName = user?.name ?? ""
  const savedEmail = user?.email ?? ""
  const savedAvatar = user?.avatar_url

  const [name, setName] = React.useState(savedName)
  const [email, setEmail] = React.useState(savedEmail)
  const [avatarUrl, setAvatarUrl] = React.useState<string | undefined>(savedAvatar)

  const [pendingEmail, setPendingEmail] = React.useState<string | null>(null)
  const [code, setCode] = React.useState("")
  const [isResending, setIsResending] = React.useState(false)

  React.useEffect(() => {
    setName(savedName)
    setEmail(savedEmail)
    setAvatarUrl(savedAvatar)
  }, [savedName, savedEmail, savedAvatar])

  const updateProfile = $api.useMutation("patch", "/auth/me")
  const confirmEmail = $api.useMutation("post", "/auth/me/confirm-email")

  const nameDirty = name.trim().length > 0 && name.trim() !== savedName
  const emailDirty = email.trim() !== savedEmail && email.trim().length > 0
  const avatarDirty = avatarUrl !== savedAvatar
  const hasChanges = nameDirty || emailDirty || avatarDirty
  const isSaving = updateProfile.isPending
  const isOAuth = oauthProviders.length > 0
  const oauthLabel = isOAuth
    ? `Managed by your ${oauthProviders.map((p) => p.charAt(0).toUpperCase() + p.slice(1)).join("/")} account`
    : undefined

  function handleSave() {
    if (!hasChanges || isSaving) return

    const body: Record<string, string> = {}
    if (nameDirty) body.name = name.trim()
    if (emailDirty && !isOAuth) body.email = email.trim()
    if (avatarDirty && avatarUrl) body.avatar_url = avatarUrl

    updateProfile.mutate(
      { body: body as never },
      {
        onSuccess: (data) => {
          const response = data as Record<string, unknown>
          if (response.status === "verification_sent") {
            setPendingEmail(email.trim())
            setCode("")
            toast.success((response.message as string) ?? "Verification code sent")
          } else {
            toast.success("Profile updated")
            queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
          }
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to update profile"))
        },
      }
    )
  }

  function handleConfirmEmail() {
    if (!code.trim() || confirmEmail.isPending) return

    confirmEmail.mutate(
      { body: { token: code.trim() } } as never,
      {
        onSuccess: () => {
          toast.success("Email address updated")
          setPendingEmail(null)
          setCode("")
          queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to confirm email"))
        },
      }
    )
  }

  function handleResendCode() {
    if (isResending || !pendingEmail) return
    setIsResending(true)
    updateProfile.mutate(
      { body: { email: pendingEmail } as never },
      {
        onSuccess: () => {
          toast.success(`Verification code resent to ${pendingEmail}`)
          setIsResending(false)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to resend code"))
          setIsResending(false)
        },
      }
    )
  }

  function handleCancelEmailChange() {
    setPendingEmail(null)
    setCode("")
    setEmail(savedEmail)
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter" && hasChanges && !isSaving) {
      event.preventDefault()
      handleSave()
    }
    if (event.key === "Escape") {
      setName(savedName)
      setEmail(savedEmail)
      setAvatarUrl(savedAvatar)
    }
  }

  return (
    <div className="flex flex-col gap-10">
      {/* Display name */}
      <section className="flex flex-col gap-2.5">
        <div>
          <Label htmlFor="display-name" className="text-[13px] font-medium">
            Display name
          </Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Your name as displayed to other members.
          </p>
        </div>
        <div className="relative max-w-sm">
          <Input
            id="display-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            onKeyDown={handleKeyDown}
            disabled={isSaving}
            className="pr-10"
          />
          {nameDirty || isSaving ? (
            <button
              type="button"
              onClick={handleSave}
              disabled={!hasChanges || isSaving}
              aria-label="Save display name"
              className="absolute top-1/2 right-1 flex size-7 -translate-y-1/2 items-center justify-center rounded-full bg-primary text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-60"
            >
              <HugeiconsIcon
                icon={isSaving ? Loading03Icon : Tick02Icon}
                strokeWidth={2.5}
                className={"size-3.5 " + (isSaving ? "animate-spin" : "")}
              />
            </button>
          ) : null}
        </div>
      </section>

      {/* Email */}
      <section className="flex flex-col gap-2.5">
        <div>
          <Label htmlFor="email" className="text-[13px] font-medium">
            Email address
          </Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            {isOAuth
              ? oauthLabel
              : "The email address associated with your account."}
          </p>
        </div>
        {pendingEmail ? (
          <div className="max-w-sm space-y-3 rounded-lg border border-border bg-muted/30 p-4">
            <p className="text-[13px] text-muted-foreground">
              Verification code sent to{" "}
              <span className="font-medium text-foreground">{pendingEmail}</span>
            </p>
            <div className="flex gap-2">
              <Input
                value={code}
                onChange={(event) => setCode(event.target.value)}
                placeholder="Enter verification code"
                disabled={confirmEmail.isPending}
                className="flex-1"
              />
              <Button
                type="button"
                onClick={handleConfirmEmail}
                disabled={!code.trim() || confirmEmail.isPending}
                size="sm"
              >
                {confirmEmail.isPending ? (
                  <HugeiconsIcon
                    icon={Loading03Icon}
                    strokeWidth={2}
                    className="size-4 animate-spin"
                  />
                ) : (
                  "Confirm"
                )}
              </Button>
            </div>
            <div className="flex gap-3 text-[12px]">
              <button
                type="button"
                onClick={handleResendCode}
                disabled={isResending}
                className="text-primary underline-offset-2 hover:underline disabled:opacity-50"
              >
                {isResending ? "Resending..." : "Resend code"}
              </button>
              <button
                type="button"
                onClick={handleCancelEmailChange}
                className="text-muted-foreground underline-offset-2 hover:underline"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <div className="relative max-w-sm">
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              onKeyDown={handleKeyDown}
              disabled={isSaving || isOAuth}
              className="pr-10"
              title={oauthLabel}
            />
            {isOAuth ? null : emailDirty || isSaving ? (
              <button
                type="button"
                onClick={handleSave}
                disabled={!emailDirty || isSaving}
                aria-label="Save email"
                className="absolute top-1/2 right-1 flex size-7 -translate-y-1/2 items-center justify-center rounded-full bg-primary text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-60"
              >
                <HugeiconsIcon
                  icon={isSaving ? Loading03Icon : Tick02Icon}
                  strokeWidth={2.5}
                  className={"size-3.5 " + (isSaving ? "animate-spin" : "")}
                />
              </button>
            ) : null}
          </div>
        )}
      </section>

      {/* Avatar */}
      <section className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <Label className="text-[13px] font-medium">Avatar</Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Upload a profile picture to personalize your account. Square. Up to 5
            MB.
          </p>
        </div>
        <ImagePicker
          assetType="avatar"
          value={avatarUrl}
          onChange={(url) => {
            setAvatarUrl(url)
          }}
          fallback={savedName?.[0]?.toUpperCase() ?? "?"}
          ariaLabel={avatarUrl ? "Replace avatar" : "Upload avatar"}
        />
      </section>

      {/* Save button */}
      <section>
        <Button
          type="button"
          onClick={handleSave}
          disabled={!hasChanges || isSaving}
        >
          {isSaving ? (
            <>
              <HugeiconsIcon
                icon={Loading03Icon}
                strokeWidth={2}
                className="mr-1.5 size-4 animate-spin"
              />
              Saving...
            </>
          ) : (
            "Save changes"
          )}
        </Button>
      </section>
    </div>
  )
}
