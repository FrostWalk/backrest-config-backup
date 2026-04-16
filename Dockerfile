FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG REVISION=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
      -ldflags="-s -w \
        -X github.com/FrostWalk/backrest-config-backup/internal/version.Version=${VERSION} \
        -X github.com/FrostWalk/backrest-config-backup/internal/version.Revision=${REVISION} \
        -X github.com/FrostWalk/backrest-config-backup/internal/version.BuildDate=${BUILD_DATE}" \
      -o /out/agent ./cmd/agent

FROM alpine:latest

ARG VERSION=dev
ARG REVISION=unknown

LABEL org.opencontainers.image.title="backrest-config-backup"
LABEL org.opencontainers.image.description="Encrypted backups of Backrest config.json to S3-compatible storage."
LABEL org.opencontainers.image.licenses=MIT
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${REVISION}"
LABEL org.opencontainers.image.os=linux
LABEL org.opencontainers.image.architecture=amd64

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/agent /usr/local/bin/agent

USER root

ENTRYPOINT ["/usr/local/bin/agent"]
