import type { components } from "@/lib/api/schema"

export type SkillRow = components["schemas"]["skillResponse"]

export function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}
