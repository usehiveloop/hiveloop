import type { components } from "@/api/schema";

export type IntegrationResponse = components["schemas"]["integrationResponse"];
export type ConnectionResponse = components["schemas"]["integConnResponse"];

export interface NangoProvider {
  name: string;
  display_name: string;
  auth_mode: string;
  webhook_user_defined_secret?: boolean;
}

export type ModalState = "closed" | "create" | "edit" | "delete-confirm" | "success";

export function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function relativeTime(dateStr: string): string {
  const diff = new Date(dateStr).getTime() - Date.now();
  const absDiff = Math.abs(diff);
  const isPast = diff < 0;

  if (absDiff < 60 * 1000) return "just now";
  if (absDiff < 60 * 60 * 1000) {
    const mins = Math.round(absDiff / (60 * 1000));
    return isPast ? `${mins}m ago` : `in ${mins}m`;
  }
  if (absDiff < 24 * 60 * 60 * 1000) {
    const hours = Math.round(absDiff / (60 * 60 * 1000));
    return isPast ? `${hours}h ago` : `in ${hours}h`;
  }
  const days = Math.round(absDiff / (24 * 60 * 60 * 1000));
  return isPast ? `${days}d ago` : `in ${days}d`;
}
