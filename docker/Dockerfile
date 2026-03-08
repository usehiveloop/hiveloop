FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /proxy-bridge ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /proxy-bridge /proxy-bridge
USER nonroot:nonroot
ENTRYPOINT ["/proxy-bridge"]
