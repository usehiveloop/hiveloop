"use client";

import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { StatusBadge, type Status } from "@/components/status-badge";
import { ProviderBadge } from "@/components/provider-badge";
import { RemainingBar } from "@/components/remaining-bar";
import { $api, fetchClient } from "@/api/client";
import type { components } from "@/api/schema";

type CredentialResponse = components["schemas"]["credentialResponse"];

function deriveStatus(cred: CredentialResponse): Status {
  if (cred.revoked_at) return "Revoked";
  if (cred.remaining != null && cred.remaining <= 0) return "Expiring";
  return "Active";
}

function formatDateTime(dateStr: string): string {
  return new Date(dateStr).toLocaleString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZoneName: "short",
  });
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return n.toString();
}

function ConfigRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-[13px] text-dim">{label}</span>
      {children}
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <>
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="h-4 w-32 animate-pulse bg-secondary" />
        <div className="h-7 w-48 animate-pulse bg-secondary" />
      </header>
      <div className="flex flex-col gap-6 px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
        <div className="flex flex-col gap-5 lg:flex-row">
          <div className="flex flex-1 animate-pulse flex-col gap-4 border border-border bg-card p-4 sm:p-5">
            <div className="h-3 w-28 bg-secondary" />
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="flex items-center justify-between">
                <div className="h-4 w-24 bg-secondary" />
                <div className="h-4 w-36 bg-secondary" />
              </div>
            ))}
          </div>
          <div className="flex w-full animate-pulse flex-col gap-4 border border-border bg-card p-4 sm:p-5 lg:w-85 lg:shrink-0">
            <div className="h-3 w-20 bg-secondary" />
            <div className="h-8 w-24 bg-secondary" />
            <div className="h-1.5 w-full bg-secondary" />
            <div className="h-3 w-32 bg-secondary" />
          </div>
        </div>
      </div>
    </>
  );
}

export default function CredentialDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();

  const { data: credential, isLoading } = $api.useQuery(
    "get",
    "/v1/credentials/{id}",
    { params: { path: { id } } },
  );

  const revokeMutation = useMutation({
    mutationFn: async () => {
      const { error } = await fetchClient.DELETE("/v1/credentials/{id}", {
        params: { path: { id } },
      });
      if (error) throw new Error((error as { error?: string }).error ?? "Failed to revoke");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/credentials"] });
      router.push("/dashboard/credentials");
    },
  });

  if (isLoading) return <LoadingSkeleton />;

  if (!credential) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-16">
        <span className="text-sm text-muted-foreground">Credential not found.</span>
        <Link href="/dashboard/credentials" className="text-[13px] text-chart-2">
          Back to credentials
        </Link>
      </div>
    );
  }

  const status = deriveStatus(credential);
  const isRevoked = status === "Revoked";
  const hasRemaining = credential.remaining != null;
  const remaining = credential.remaining ?? 0;
  const max = credential.refill_amount ?? remaining;
  const percent = max > 0 ? Math.round((remaining / max) * 100) : 0;

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="flex items-center gap-1.5">
          <Link href="/dashboard/credentials" className="text-[13px] text-dim hover:text-foreground">
            Credentials
          </Link>
          <span className="text-[13px] text-dim">/</span>
          <span className="text-[13px] text-muted-foreground">{credential.label || "Untitled"}</span>
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <h1 className="font-mono text-lg font-semibold tracking-tight text-foreground sm:text-[22px]">
              {credential.label || "Untitled"}
            </h1>
            <StatusBadge status={status} />
          </div>
          {!isRevoked && (
            <div className="flex items-center gap-2">
              <Button
                variant="destructive"
                size="lg"
                onClick={() => revokeMutation.mutate()}
                loading={revokeMutation.isPending}
              >
                Revoke
              </Button>
            </div>
          )}
        </div>
      </header>

      {/* Content */}
      <div className="flex flex-col gap-6 px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
        {/* Info Cards */}
        <div className="flex flex-col gap-5 lg:flex-row">
          {/* Configuration Card */}
          <div className="flex flex-1 flex-col gap-4 border border-border bg-card p-4 sm:p-5">
            <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">Configuration</span>
            <div className="flex flex-col gap-3.5">
              <ConfigRow label="Base URL">
                <span className="font-mono text-[13px] text-foreground">{credential.base_url}</span>
              </ConfigRow>
              <ConfigRow label="Auth Scheme">
                <span className="font-mono text-[13px] text-foreground">{credential.auth_scheme}</span>
              </ConfigRow>
              {credential.provider_id && (
                <ConfigRow label="Provider">
                  <ProviderBadge provider={credential.provider_id} />
                </ConfigRow>
              )}
              {credential.identity_id && (
                <ConfigRow label="Identity">
                  <Link
                    href={`/dashboard/identities/${credential.identity_id}`}
                    className="font-mono text-[13px] text-chart-2"
                  >
                    {credential.identity_id}
                  </Link>
                </ConfigRow>
              )}
              <ConfigRow label="Created">
                <span className="font-mono text-[13px] text-muted-foreground">
                  {credential.created_at ? formatDateTime(credential.created_at) : "—"}
                </span>
              </ConfigRow>
              {credential.revoked_at && (
                <ConfigRow label="Revoked">
                  <span className="font-mono text-[13px] text-destructive">
                    {formatDateTime(credential.revoked_at)}
                  </span>
                </ConfigRow>
              )}
              <ConfigRow label="ID">
                <span className="font-mono text-[13px] text-muted-foreground">{credential.id}</span>
              </ConfigRow>
            </div>
          </div>

          {/* Usage Card */}
          <div className="flex w-full flex-col gap-4 border border-border bg-card p-4 sm:p-5 lg:w-85 lg:shrink-0">
            <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">Usage</span>

            {hasRemaining ? (
              <div className="flex flex-col gap-2">
                <span className="font-mono text-[28px] font-medium leading-8.5 tracking-tight text-foreground">
                  {formatCount(remaining)}
                </span>
                <span className="text-xs text-dim">of {formatCount(max)}</span>
                <RemainingBar current={formatCount(remaining)} max={formatCount(max)} percent={percent} />
                <span className="text-[11px] text-dim">remaining this period</span>
              </div>
            ) : (
              <div className="flex flex-col gap-1">
                <span className="font-mono text-[28px] font-medium leading-8.5 tracking-tight text-foreground">
                  Unlimited
                </span>
                <span className="text-xs text-dim">no request cap configured</span>
              </div>
            )}

            <div className="flex flex-col gap-3 border-t border-border pt-4">
              {credential.refill_amount != null && (
                <ConfigRow label="Refill Amount">
                  <span className="font-mono text-[13px] text-foreground">
                    {formatCount(credential.refill_amount)}
                  </span>
                </ConfigRow>
              )}
              {credential.refill_interval && (
                <ConfigRow label="Refill Interval">
                  <span className="font-mono text-[13px] text-foreground">
                    {credential.refill_interval}
                  </span>
                </ConfigRow>
              )}
              <ConfigRow label="Total Requests">
                <span className="font-mono text-[13px] text-foreground">
                  {formatCount(credential.request_count ?? 0)}
                </span>
              </ConfigRow>
              <ConfigRow label="Last Used">
                <span className="font-mono text-[13px] text-muted-foreground">
                  {credential.last_used_at ? formatDateTime(credential.last_used_at) : "Never"}
                </span>
              </ConfigRow>
            </div>
          </div>
        </div>

        {/* Metadata */}
        {credential.meta && Object.keys(credential.meta).length > 0 && (
          <div className="flex flex-col">
            <div className="pb-4">
              <span className="text-sm font-medium text-foreground">Metadata</span>
            </div>
            <div className="border border-border bg-code p-4">
              <pre className="font-mono text-xs leading-5 text-muted-foreground">
                {JSON.stringify(credential.meta, null, 2)}
              </pre>
            </div>
          </div>
        )}
      </div>
    </>
  );
}
