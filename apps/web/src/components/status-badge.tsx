import { Badge } from "@/components/ui/badge";

export type Status = "Active" | "Expiring" | "Revoked";

const statusConfig: Record<Status, string> = {
  Active: "border-success/20 bg-success/10 text-success-foreground",
  Expiring: "border-warning/20 bg-warning/10 text-warning-foreground",
  Revoked: "border-destructive/20 bg-destructive/10 text-destructive",
};

export function StatusBadge({ status }: { status: Status }) {
  return (
    <Badge variant="outline" className={`h-auto text-[11px] ${statusConfig[status]}`}>
      {status}
    </Badge>
  );
}

export function StatusCodeBadge({ code }: { code: number }) {
  let colors: string;
  if (code >= 200 && code < 300) colors = "border-success/20 bg-success/10 text-success-foreground";
  else if (code >= 400 && code < 500) colors = "border-warning/20 bg-warning/10 text-warning-foreground";
  else colors = "border-destructive/20 bg-destructive/10 text-destructive";

  return (
    <Badge variant="outline" className={`h-auto w-9 justify-center font-mono text-2xs ${colors}`}>
      {code}
    </Badge>
  );
}
