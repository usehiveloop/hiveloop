"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { Lock, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { $api } from "@/api/client";

export function SetupWorkspace() {
  const router = useRouter();
  const started = useRef(false);

  const createOrg = $api.useMutation("post", "/v1/orgs", {
    onSuccess() {
      router.push("/sign-in");
    },
  });

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    createOrg.mutate({ body: { name: "My Organization" } });
  }, [createOrg]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="flex flex-col items-center gap-6 text-center">
        <div className="flex size-12 items-center justify-center border border-primary bg-primary/20">
          <Lock className="size-6 text-primary" />
        </div>
        <div className="flex flex-col gap-2">
          <h1
            className="text-2xl font-semibold tracking-tight text-foreground"
            style={{ fontFamily: "var(--font-bricolage)" }}
          >
            Setting up your workspace
          </h1>
          <p className="text-sm text-muted-foreground">
            {createOrg.isError
              ? "Something went wrong. Please try again."
              : "We're creating your organization. This will only take a moment."}
          </p>
        </div>
        {createOrg.isError ? (
          <Button onClick={() => createOrg.mutate({ body: { name: "My Organization" } })}>
            Try again
          </Button>
        ) : (
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        )}
      </div>
    </div>
  );
}
