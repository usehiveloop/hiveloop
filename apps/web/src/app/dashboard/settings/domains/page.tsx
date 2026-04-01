"use client";

import { useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { $api } from "@/api/client";
import { useQueryClient } from "@tanstack/react-query";
import { Check, Clock, Copy, Globe, Trash2, RefreshCw, CircleAlert } from "lucide-react";

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [text]);

  return (
    <button
      onClick={handleCopy}
      className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
    >
      {copied ? <Check className="size-3 text-green-500" /> : <Copy className="size-3" />}
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

export default function DomainsSettingsPage() {
  const queryClient = useQueryClient();
  const [newDomain, setNewDomain] = useState("");
  const [verifyResults, setVerifyResults] = useState<
    Record<string, { verified: boolean; message: string }>
  >({});

  const { data: domains = [], isLoading } = $api.useQuery("get", "/v1/custom-domains");

  const createMutation = $api.useMutation("post", "/v1/custom-domains", {
    onSuccess: () => {
      setNewDomain("");
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/custom-domains"] });
    },
  });

  const verifyMutation = $api.useMutation("post", "/v1/custom-domains/{id}/verify", {
    onSuccess: (data, { params }) => {
      const id = params.path.id;
      setVerifyResults((prev) => ({
        ...prev,
        [id]: { verified: data.verified ?? false, message: data.message ?? "" },
      }));
      if (data.verified) {
        queryClient.invalidateQueries({ queryKey: ["get", "/v1/custom-domains"] });
      }
    },
  });

  const deleteMutation = $api.useMutation("delete", "/v1/custom-domains/{id}", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/custom-domains"] });
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newDomain.trim()) return;
    createMutation.mutate({ body: { domain: newDomain.trim() } });
  };

  return (
    <div className="flex flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
      {/* Add Domain */}
      <div className="flex flex-col gap-4 border border-border bg-card p-5">
        <div className="flex flex-col gap-1">
          <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">
            Add Custom Domain
          </span>
          <p className="text-[13px] text-muted-foreground">
            Configure a custom domain for sandbox preview URLs. You&apos;ll need to create two DNS
            records to verify ownership and enable TLS.
          </p>
        </div>

        <form onSubmit={handleSubmit} className="flex items-center gap-3">
          <Input
            placeholder="preview.yourdomain.com"
            value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)}
            className="max-w-sm text-[13px]"
          />
          <Button size="lg" type="submit" loading={createMutation.isPending}>
            Add Domain
          </Button>
        </form>

        {createMutation.error && (
          <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
            <CircleAlert className="size-3.5 shrink-0 text-destructive" />
            <span className="text-xs text-destructive">
              {(createMutation.error as Error).message}
            </span>
          </div>
        )}
      </div>

      {/* Domain List */}
      {isLoading ? (
        <div className="flex items-center justify-center py-12 text-[13px] text-muted-foreground">
          Loading domains...
        </div>
      ) : !domains || domains.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-2 border border-dashed border-border py-12">
          <Globe className="size-8 text-muted-foreground/50" />
          <p className="text-[13px] text-muted-foreground">No custom domains configured</p>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          {domains.map((domain) => {
            const domainId = domain.id ?? "";
            const isVerified = domain.verified ?? false;
            const verifyResult = verifyResults[domainId];

            return (
              <div
                key={domainId}
                className="flex flex-col gap-4 border border-border bg-card p-5"
              >
                {/* Header */}
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <Globe className="size-4 text-muted-foreground" />
                    <span className="font-mono text-[13px] font-medium text-foreground">
                      *.{domain.domain}
                    </span>
                    {isVerified ? (
                      <Badge
                        variant="outline"
                        className="gap-1 border-green-500/30 text-green-500"
                      >
                        <Check className="size-3" />
                        Verified
                      </Badge>
                    ) : (
                      <Badge
                        variant="outline"
                        className="gap-1 border-yellow-500/30 text-yellow-500"
                      >
                        <Clock className="size-3" />
                        Pending
                      </Badge>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    {!isVerified && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() =>
                          verifyMutation.mutate({
                            params: { path: { id: domainId } },
                          })
                        }
                        loading={
                          verifyMutation.isPending &&
                          verifyMutation.variables?.params?.path?.id === domainId
                        }
                      >
                        <RefreshCw className="mr-1.5 size-3" />
                        Verify
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        deleteMutation.mutate({
                          params: { path: { id: domainId } },
                        })
                      }
                      loading={
                        deleteMutation.isPending &&
                        deleteMutation.variables?.params?.path?.id === domainId
                      }
                      className="text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                </div>

                {/* Verify result message */}
                {verifyResult && (
                  <div
                    className={`flex items-center gap-2 px-3 py-2.5 text-xs ${
                      verifyResult.verified
                        ? "border border-green-500/20 bg-green-500/5 text-green-500"
                        : "border border-yellow-500/20 bg-yellow-500/5 text-yellow-500"
                    }`}
                  >
                    {verifyResult.verified ? (
                      <Check className="size-3.5 shrink-0" />
                    ) : (
                      <CircleAlert className="size-3.5 shrink-0" />
                    )}
                    {verifyResult.message}
                  </div>
                )}

                {/* DNS Records */}
                <div className="flex flex-col gap-2">
                  <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">
                    Required DNS Records
                  </span>
                  <div className="flex flex-col gap-2">
                    {(domain.dns_records ?? []).map(
                      (record: { type?: string; name?: string; value?: string }, i: number) => (
                        <div
                          key={i}
                          className="flex items-start justify-between gap-4 border border-border bg-background px-3 py-2.5"
                        >
                          <div className="flex flex-col gap-0.5 overflow-hidden">
                            <div className="flex items-center gap-2">
                              <Badge
                                variant="secondary"
                                className="shrink-0 font-mono text-[10px]"
                              >
                                {record.type}
                              </Badge>
                              <span className="truncate font-mono text-[12px] text-foreground">
                                {record.name}
                              </span>
                            </div>
                            <span className="truncate font-mono text-[12px] text-muted-foreground">
                              {record.value}
                            </span>
                          </div>
                          <CopyButton text={record.value ?? ""} />
                        </div>
                      )
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
