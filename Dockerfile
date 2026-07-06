# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder
WORKDIR /src/web

COPY web/package*.json ./
RUN --mount=type=cache,target=/root/.npm,sharing=locked npm ci

COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend-builder
ARG TARGETOS=linux
ARG TARGETARCH=arm64
ARG VERSION=dev
ARG BUILD_TIME=unknown

WORKDIR /src
ENV CGO_ENABLED=0 \
    GOWORK=off

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
COPY third_party/vowifi-go ./third_party/vowifi-go

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

COPY . ./
COPY --from=frontend-builder /src/web/dist ./internal/web/dist
RUN test -s internal/web/dist/index.html

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false -tags "with_utls nomsgpack" \
      -ldflags "-s -w -X 'github.com/iniwex5/vohive/internal/global.Version=${VERSION}' -X 'github.com/iniwex5/vohive/internal/global.BuildTime=${BUILD_TIME}'" \
      -o /out/vo-hive ./cmd/vohive

FROM alpine:3.22
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata iproute2 psmisc && \
    mkdir -p /app/config /app/data /app/logs

COPY --from=backend-builder /out/vo-hive /app/vo-hive
COPY entrypoint.sh /app/entrypoint.sh
COPY config/config.yaml.example /app/config.yaml.example
RUN chmod +x /app/entrypoint.sh

ENV TZ=Asia/Shanghai \
    CONFIG_PATH=/app/config/config.yaml

EXPOSE 7575
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["-c", "/app/config/config.yaml"]
