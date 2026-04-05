"use client"

import { useState } from "react"
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { HugeiconsIcon } from "@hugeicons/react"
import { Copy01Icon } from "@hugeicons/core-free-icons"
import { toast } from "sonner"

interface ConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel?: string
  loading?: boolean
  destructive?: boolean
  onConfirm: () => void
  confirmText?: string
  confirmTextLabel?: string
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = "Confirm",
  loading = false,
  destructive = false,
  onConfirm,
  confirmText,
  confirmTextLabel,
}: ConfirmDialogProps) {
  const [typedValue, setTypedValue] = useState("")
  const requiresTyping = !!confirmText
  const isConfirmEnabled = !requiresTyping || typedValue === confirmText

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) setTypedValue("")
    onOpenChange(nextOpen)
  }

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>

        {requiresTyping && (
          <div className="flex flex-col gap-2">
            <Label className="text-sm">
              {confirmTextLabel ?? (
                <>
                  Type{" "}
                  <button
                    type="button"
                    onClick={() => {
                      navigator.clipboard.writeText(confirmText as string)
                      toast.success("Copied to clipboard")
                    }}
                    className="inline-flex items-center gap-1 font-semibold text-foreground cursor-pointer"
                  >
                    {confirmText}
                    <HugeiconsIcon icon={Copy01Icon} size={12} className="text-muted-foreground" />
                  </button>
                  {" "}to confirm
                </>
              )}
            </Label>
            <Input
              value={typedValue}
              onChange={(event) => setTypedValue(event.target.value)}
              placeholder={confirmText}
              autoComplete="off"
              autoFocus
            />
          </div>
        )}

        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <Button
            variant={destructive ? "destructive" : "default"}
            loading={loading}
            disabled={!isConfirmEnabled}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
