"use client"

export function TimeAgo({ date }: { date: string | undefined | null }) {
  if (!date) return <span className="text-muted-foreground">--</span>

  const d = new Date(date)
  const now = Date.now()
  const diff = now - d.getTime()

  const seconds = Math.floor(diff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  let label: string
  if (days > 30) label = d.toLocaleDateString()
  else if (days > 0) label = `${days}d ago`
  else if (hours > 0) label = `${hours}h ago`
  else if (minutes > 0) label = `${minutes}m ago`
  else label = "just now"

  return (
    <time dateTime={date} title={d.toLocaleString()}>
      {label}
    </time>
  )
}
