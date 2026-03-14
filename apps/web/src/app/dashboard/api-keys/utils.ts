import type { components } from "@/api/schema";
import type { Status } from "@/components/status-badge";

export type APIKeyResponse = components["schemas"]["apiKeyResponse"];
export type CreateAPIKeyResult = components["schemas"]["createAPIKeyResponse"];
export type StatusFilter = "All" | "Active" | "Expired" | "Revoked";
export type ModalState = "closed" | "create" | "success" | "revoke-confirm";

export const SCOPE_OPTIONS = [
  { value: "all", label: "All" },
  { value: "connect", label: "Connect" },
  { value: "credentials", label: "Credentials" },
  { value: "tokens", label: "Tokens" },
];

export const EXPIRY_OPTIONS = [
  { value: "", label: "Never" },
  { value: "720h", label: "30 days" },
  { value: "2160h", label: "90 days" },
  { value: "8760h", label: "1 year" },
];

export function deriveStatus(key: APIKeyResponse): Status {
  if (key.revoked_at) return "Revoked";
  if (key.expires_at && new Date(key.expires_at) < new Date()) return "Revoked";
  if (key.expires_at) {
    const hoursLeft = (new Date(key.expires_at).getTime() - Date.now()) / (1000 * 60 * 60);
    if (hoursLeft < 24) return "Expiring";
  }
  return "Active";
}

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
