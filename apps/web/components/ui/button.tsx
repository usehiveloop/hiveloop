"use client"

import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon } from "@hugeicons/core-free-icons"

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "group/button relative inline-flex shrink-0 items-center justify-center rounded-md border border-transparent bg-clip-padding text-sm font-semibold whitespace-nowrap transition-all duration-200 outline-none select-none cursor-pointer focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 disabled:opacity-50 disabled:cursor-not-allowed aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground shadow-[0_0_16px_rgba(0,0,0,0.08)] dark:shadow-[0_0_20px_rgba(255,77,109,0.25)] hover:scale-105",
        outline:
          "border-border bg-transparent text-foreground hover:bg-black/[0.03] dark:hover:bg-white/[0.05]",
        secondary:
          "bg-secondary border border-border text-secondary-foreground hover:scale-105",
        ghost:
          "bg-transparent text-muted-foreground hover:underline hover:underline-offset-4",
        destructive:
          "bg-destructive/10 text-destructive hover:bg-destructive/20 focus-visible:border-destructive/40 focus-visible:ring-destructive/20 dark:bg-destructive/20 dark:hover:bg-destructive/30 dark:focus-visible:ring-destructive/40",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-11 px-5 text-sm",
        xs: "h-6 gap-1 px-2.5 text-xs",
        sm: "h-9 px-4 text-xs",
        lg: "h-12 px-8 text-sm",
        icon: "size-9",
        "icon-xs": "size-6 [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-8",
        "icon-lg": "size-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  loading,
  children,
  ...props
}: ButtonPrimitive.Props &
  VariantProps<typeof buttonVariants> & { loading?: boolean }) {
  const isDisabled = loading || props.disabled
  return (
    <ButtonPrimitive
      data-slot="button"
      disabled={isDisabled}
      className={cn(
        buttonVariants({ variant, size, className }),
        isDisabled && (variant === "default" || variant === "secondary") && "hover:scale-100"
      )}
      {...props}
    >
      {loading ? (
        <>
          <span className="invisible contents">{children}</span>
          <HugeiconsIcon icon={Loading03Icon} className="absolute size-4 animate-spin" />
        </>
      ) : (
        children
      )}
    </ButtonPrimitive>
  )
}

export { Button, buttonVariants }
