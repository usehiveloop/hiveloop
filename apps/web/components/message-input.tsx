"use client"

import { useState } from "react"
import { Textarea } from "@/components/ui/textarea"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { SentIcon } from "@hugeicons/core-free-icons"
import { cn } from "@/lib/utils"

type MessageInputProps = {
  placeholder?: string
  onSend?: (message: string) => void
  disabled?: boolean
  className?: string
}

export function MessageInput({
  placeholder = "Type a message...",
  onSend,
  disabled,
  className,
}: MessageInputProps) {
  const [value, setValue] = useState("")

  function handleSend() {
    if (!value.trim()) return
    onSend?.(value.trim())
    setValue("")
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  return (
    <div className={cn("relative", className)}>
      <Textarea
        placeholder={placeholder}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        disabled={disabled}
        className="pr-12 min-h-20 max-h-40"
      />
      <Button
        size="icon-sm"
        disabled={!value.trim() || disabled}
        onClick={handleSend}
        className="absolute bottom-2 right-2"
      >
        <HugeiconsIcon icon={SentIcon} size={14} />
      </Button>
    </div>
  )
}
