FROM docker.io/library/golang:1.26.1-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder

ARG VERSION=development
ARG REVISION=unknown

# hadolint ignore=DL3018
RUN echo 'nobody:x:65534:65534:Nobody:/home/nobody:' > /tmp/passwd && \
    apk add --no-cache ca-certificates && \
    mkdir -p /home/nobody/.mcp-tg && chown 65534:65534 /home/nobody/.mcp-tg

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.revision=${REVISION}" -trimpath ./cmd/mcp-tg

FROM scratch

COPY --from=builder /tmp/passwd /etc/passwd
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder --chmod=555 /build/mcp-tg /mcp-tg
COPY --from=builder --chown=65534:65534 /home/nobody/.mcp-tg /home/nobody/.mcp-tg

ENV TELEGRAM_SESSION_FILE=/home/nobody/.mcp-tg/session.json

USER 65534
ENTRYPOINT ["/mcp-tg"]
