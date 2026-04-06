"use client"

import * as React from "react"
import { Dialog as PanelPrimitive } from "@base-ui/react/dialog"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { Cancel01Icon } from "@hugeicons/core-free-icons"

function FloatingPanel({ ...props }: PanelPrimitive.Root.Props) {
  return <PanelPrimitive.Root data-slot="floating-panel" {...props} />
}

function FloatingPanelTrigger({ ...props }: PanelPrimitive.Trigger.Props) {
  return <PanelPrimitive.Trigger data-slot="floating-panel-trigger" {...props} />
}

function FloatingPanelClose({ ...props }: PanelPrimitive.Close.Props) {
  return <PanelPrimitive.Close data-slot="floating-panel-close" {...props} />
}

function FloatingPanelPortal({ ...props }: PanelPrimitive.Portal.Props) {
  return <PanelPrimitive.Portal data-slot="floating-panel-portal" {...props} />
}

function FloatingPanelOverlay({
  className,
  ...props
}: PanelPrimitive.Backdrop.Props) {
  return (
    <PanelPrimitive.Backdrop
      data-slot="floating-panel-overlay"
      className={cn(
        "fixed inset-0 z-50 bg-black/30 transition-opacity duration-200 data-ending-style:opacity-0 data-starting-style:opacity-0 supports-backdrop-filter:backdrop-blur-sm",
        className
      )}
      {...props}
    />
  )
}

interface FloatingPanelContentProps extends PanelPrimitive.Popup.Props {
  showCloseButton?: boolean
  width?: string
}

function FloatingPanelContent({
  className,
  children,
  showCloseButton = true,
  width = "lg:w-[580px]",
  ...props
}: FloatingPanelContentProps) {
  return (
    <FloatingPanelPortal>
      <FloatingPanelOverlay />
      <PanelPrimitive.Popup
        data-slot="floating-panel-content"
        className={cn(
          "fixed inset-4 z-50 flex flex-col rounded-2xl border border-border bg-background shadow-2xl shadow-black/20 transition-all duration-200 data-ending-style:opacity-0 data-ending-style:translate-x-4 data-starting-style:opacity-0 data-starting-style:translate-x-4 sm:inset-6 lg:inset-y-6 lg:left-auto lg:right-6",
          width,
          className
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <PanelPrimitive.Close
            data-slot="floating-panel-close"
            render={
              <Button
                variant="ghost"
                className="absolute top-4 right-4"
                size="icon-sm"
              />
            }
          >
            <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
            <span className="sr-only">Close</span>
          </PanelPrimitive.Close>
        )}
      </PanelPrimitive.Popup>
    </FloatingPanelPortal>
  )
}

function FloatingPanelHeader({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="floating-panel-header"
      className={cn("flex flex-col gap-1.5 border-b border-border p-6", className)}
      {...props}
    />
  )
}

function FloatingPanelBody({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="floating-panel-body"
      className={cn("flex-1 overflow-y-auto p-6", className)}
      {...props}
    />
  )
}

function FloatingPanelFooter({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="floating-panel-footer"
      className={cn(
        "mt-auto flex items-center gap-2 border-t border-border p-6",
        className
      )}
      {...props}
    />
  )
}

function FloatingPanelTitle({
  className,
  ...props
}: PanelPrimitive.Title.Props) {
  return (
    <PanelPrimitive.Title
      data-slot="floating-panel-title"
      className={cn(
        "font-heading text-base font-medium text-foreground",
        className
      )}
      {...props}
    />
  )
}

function FloatingPanelDescription({
  className,
  ...props
}: PanelPrimitive.Description.Props) {
  return (
    <PanelPrimitive.Description
      data-slot="floating-panel-description"
      className={cn("text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

export {
  FloatingPanel,
  FloatingPanelTrigger,
  FloatingPanelClose,
  FloatingPanelPortal,
  FloatingPanelOverlay,
  FloatingPanelContent,
  FloatingPanelHeader,
  FloatingPanelBody,
  FloatingPanelFooter,
  FloatingPanelTitle,
  FloatingPanelDescription,
}
