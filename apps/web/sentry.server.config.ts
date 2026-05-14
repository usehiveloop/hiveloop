// This file configures the initialization of Sentry on the server.
// The config you add here will be used whenever the server handles a request.
// https://docs.sentry.io/platforms/javascript/guides/nextjs/

import * as Sentry from "@sentry/nextjs";

const tracesSampleRate = Number(process.env.SENTRY_TRACES_SAMPLE_RATE ?? "0.01");

Sentry.init({
  dsn: process.env.SENTRY_DSN ?? process.env.NEXT_PUBLIC_SENTRY_DSN,

  tracesSampleRate: Number.isFinite(tracesSampleRate) ? tracesSampleRate : 0.01,
  enableLogs: process.env.SENTRY_ENABLE_LOGS === "true",
  sendDefaultPii: false,
});
