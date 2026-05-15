export function formatLabel(value?: string, fallback = "Unassigned") {
  if (!value) return fallback
  return value
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}

export function formatCategoryLabel(category?: string) {
  if (category === "engineering") return "Software engineering"
  return formatLabel(category)
}
