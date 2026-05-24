import pino from "pino"

export const log = pino({
  level: process.env.HIVY_LOG_LEVEL ?? "info",
  ...(process.env.NODE_ENV === "development"
    ? { transport: { target: "pino/file", options: { destination: 1 } } }
    : {}),
})
