import { Badge } from "@/components/ui/badge"

const variantMap: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  active: "default",
  running: "default",
  ready: "default",
  verified: "default",
  completed: "secondary",
  stopped: "outline",
  archived: "outline",
  ended: "outline",
  pending: "secondary",
  building: "secondary",
  error: "destructive",
  failed: "destructive",
  cancelled: "outline",
  revoked: "destructive",
  banned: "destructive",
}

export function StatusBadge({ status }: { status: string }) {
  const variant = variantMap[status] ?? "outline"
  return (
    <Badge variant={variant} className="capitalize">
      {status}
    </Badge>
  )
}
