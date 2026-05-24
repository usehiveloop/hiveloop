FROM node:22-alpine AS deps

WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1

RUN corepack enable && corepack prepare pnpm@10.18.2 --activate

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

FROM node:22-alpine AS build

WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1

ARG NEXT_PUBLIC_HIVY_API_URL
ARG NEXT_PUBLIC_HIVY_CONNECTIONS_HOST
ARG NEXT_PUBLIC_HIVY_SENTRY_DSN
ARG NEXT_PUBLIC_HIVY_SENTRY_TRACES_SAMPLE_RATE

ENV NEXT_PUBLIC_HIVY_API_URL=$NEXT_PUBLIC_HIVY_API_URL
ENV NEXT_PUBLIC_HIVY_CONNECTIONS_HOST=$NEXT_PUBLIC_HIVY_CONNECTIONS_HOST
ENV NEXT_PUBLIC_HIVY_SENTRY_DSN=$NEXT_PUBLIC_HIVY_SENTRY_DSN
ENV NEXT_PUBLIC_HIVY_SENTRY_TRACES_SAMPLE_RATE=$NEXT_PUBLIC_HIVY_SENTRY_TRACES_SAMPLE_RATE

RUN corepack enable && corepack prepare pnpm@10.18.2 --activate

COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN pnpm build

FROM node:22-alpine AS runner

WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3000

RUN corepack enable && corepack prepare pnpm@10.18.2 --activate

COPY --from=build /app ./

EXPOSE 3000

CMD ["pnpm", "exec", "next", "start", "-p", "3000", "-H", "0.0.0.0"]
