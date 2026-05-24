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
    -o /hivy ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.21

ARG VERSION=dev
ARG COMMIT=unknown

LABEL org.opencontainers.image.source="https://github.com/usehivy/hivy"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"

RUN apk add --no-cache ca-certificates tzdata nginx

COPY --from=build /hivy /hivy
COPY --from=build /src/global /global
COPY proxy.nginx.conf /etc/nginx/http.d/default.conf

EXPOSE 80 8080

CMD ["sh", "-c", "nginx && exec /hivy serve"]
