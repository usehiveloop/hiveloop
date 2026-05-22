"use client"

import Link from "next/link"
import { motion } from "motion/react"
import { useForm } from "react-hook-form"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  usePasswordLogin,
  type PasswordAuthInput,
} from "@/hooks/use-password-auth"
import {
  AuthCard,
  AuthGhostLogo,
  AuthDivider,
  AuthFooter,
  OAuthButtons,
  stepVariants,
  stepTransition,
} from "../_components/shared"

export default function LoginPage() {
  const { login, isPending } = usePasswordLogin()
  const { register, handleSubmit } = useForm<PasswordAuthInput>({
    defaultValues: {
      email: "",
      password: "",
    },
  })

  const onSubmit = handleSubmit((values) => login(values))

  return (
    <>
      <div className="fixed top-5 right-0 left-0 z-50 mx-auto flex max-w-5xl items-center px-4 md:px-0">
        <Button
          variant="ghost"
          size="sm"
          render={<Link href="/" className="flex items-center font-display" />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} size={14} />
          Home
        </Button>
      </div>

      <AuthCard>
        <div className="flex flex-col gap-8">
          {/* Header */}
          <div className="flex flex-col items-center gap-4">
            <AuthGhostLogo />
            <div className="text-center">
              <h1 className="font-heading text-2xl font-normal tracking-[-0.02em] text-foreground">
                Sign in to hivy
              </h1>
              <p className="mt-1.5 text-sm text-muted-foreground">
                Welcome back. Sign in to manage your AI coworkers.
              </p>
            </div>
          </div>

          <motion.div
            key="password-login"
            variants={stepVariants}
            initial="initial"
            animate="animate"
            exit="exit"
            transition={stepTransition}
            className="flex flex-col gap-6"
          >
            <OAuthButtons />
            <AuthDivider />
            <form onSubmit={onSubmit} className="flex flex-col gap-3">
              <div className="space-y-2">
                <Label htmlFor="email">Work email</Label>
                <Input
                  id="email"
                  type="email"
                  autoComplete="email"
                  required
                  placeholder="you@company.com"
                  {...register("email", { required: true })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  required
                  placeholder="Enter your password"
                  {...register("password", { required: true })}
                />
              </div>
              <Button
                type="submit"
                size="lg"
                className="w-full"
                loading={isPending}
              >
                Sign in
              </Button>
            </form>
            <p className="text-center text-sm text-muted-foreground">
              Don't have an account?{" "}
              <Link
                href="/auth/signup"
                className="font-medium text-foreground underline-offset-4 transition-colors hover:underline"
              >
                Sign up
              </Link>
            </p>
          </motion.div>

          {/* Footer */}
          <AuthFooter />
        </div>
      </AuthCard>
    </>
  )
}
