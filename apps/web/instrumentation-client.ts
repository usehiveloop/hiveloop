// This file configures the initialization of Sentry on the client.
// The added config here will be used whenever a users loads a page in their browser.
// https://docs.sentry.io/platforms/javascript/guides/nextjs/

import * as Sentry from "@sentry/nextjs";

const tracesSampleRate = Number(
  process.env.NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE ?? "0.01",
);

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,

  tracesSampleRate: Number.isFinite(tracesSampleRate) ? tracesSampleRate : 0.01,
  enableLogs: false,
  sendDefaultPii: false,
});

export const onRouterTransitionStart = Sentry.captureRouterTransitionStart;
