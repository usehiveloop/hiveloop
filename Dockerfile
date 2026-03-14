# ---- Build stage ----
FROM golang:1.25-alpine AS build

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /src

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /llmvault ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata nginx

COPY --from=build /llmvault /llmvault
COPY proxy.nginx.conf /etc/nginx/http.d/default.conf

EXPOSE 80 8080

CMD ["sh", "-c", "nginx && exec /llmvault"]
