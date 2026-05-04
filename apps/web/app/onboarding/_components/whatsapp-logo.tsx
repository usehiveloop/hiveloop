import { cn } from "@/lib/utils"

const sizeClasses: Record<number, string> = {
  16: "size-4",
  20: "size-5",
  24: "size-6",
  28: "size-7",
  32: "size-8",
  40: "size-10",
  48: "size-12",
}

export function WhatsappLogo({ size = 32, className }: { size?: number; className?: string }) {
  const sizeClass = sizeClasses[size] ?? "size-8"

  return (
    <div className={cn("shrink-0 rounded-md bg-white p-0.5", sizeClass, className)}>
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src="/images/whatsapp.svg" alt="WhatsApp" className="size-full" />
    </div>
  )
}
