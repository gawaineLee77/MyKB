FROM golang:1.26-alpine AS builder

WORKDIR /src/services/gateway
COPY services/gateway/go.mod services/gateway/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY services/gateway/ ./
ARG MINDCREEK_VERSION=0.5.0-phase4
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go test ./... \
    && CGO_ENABLED=0 go build \
      -ldflags "-s -w -X main.productVersion=${MINDCREEK_VERSION}" \
      -o /out/mindcreek-gateway ./cmd/gateway

FROM alpine:3.23

RUN addgroup -S mindcreek && adduser -S -G mindcreek mindcreek
COPY --from=builder /out/mindcreek-gateway /usr/local/bin/mindcreek-gateway
USER mindcreek
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=6 \
  CMD ["/usr/local/bin/mindcreek-gateway", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/mindcreek-gateway"]
