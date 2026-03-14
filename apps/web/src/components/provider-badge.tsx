import { Badge } from "@/components/ui/badge";

export function ProviderBadge({ provider }: { provider: string }) {
  return (
    <Badge variant="secondary" className="h-auto bg-[#8B5CF614] font-mono text-[11px] text-[#A78BFA]">
      {provider}
    </Badge>
  );
}
