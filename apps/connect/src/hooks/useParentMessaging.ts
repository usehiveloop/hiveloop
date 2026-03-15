import { useEffect, useCallback, useRef } from "react";
import type { ConnectEvent } from "./parentEventContextDef";

export function useParentMessaging() {
  const isEmbedded = typeof window !== "undefined" && window.parent !== window;
  const parentOriginRef = useRef<string | null>(null);

  useEffect(() => {
    if (!isEmbedded) return;

    function handleMessage(event: MessageEvent) {
      if (!parentOriginRef.current && event.source === window.parent) {
        parentOriginRef.current = event.origin;
      }
    }

    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [isEmbedded]);

  const sendToParent = useCallback(
    (message: ConnectEvent) => {
      if (!isEmbedded) return;
      const origin = parentOriginRef.current ?? "*";
      window.parent.postMessage(message, origin);
    },
    [isEmbedded],
  );

  return { sendToParent, isEmbedded };
}
