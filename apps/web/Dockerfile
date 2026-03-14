# ---- Build stage ----
FROM node:22-alpine AS build

WORKDIR /app

# Cache dependency installs
COPY package*.json ./
RUN npm ci

# Copy source and build
COPY . .

ARG NEXT_PUBLIC_API_URL
ARG NEXT_PUBLIC_LOGTO_ISSUER
ARG NEXT_PUBLIC_CONNECT_URL
ARG NEXT_PUBLIC_LOGTO_CLIENT_ID

RUN npm run build

# ---- Runtime stage ----
FROM node:22-alpine

WORKDIR /app

ENV NODE_ENV=production

COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public

USER node

EXPOSE 3000

CMD ["node", "server.js"]
