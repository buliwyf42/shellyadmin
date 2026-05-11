FROM node:20-alpine@sha256:fb4cd12c85ee03686f6af5362a0b0d56d50c58a04632e6c0fb8363f609372293 AS frontend
WORKDIR /app/web
COPY web/package*.json ./
# --ignore-scripts blocks malicious postinstall hooks in compromised npm deps;
# Svelte/Vite/TS don't require lifecycle scripts at install time. If a future
# dep needs a build hook, allowlist it via `npm rebuild <pkg>` explicitly.
RUN npm ci --ignore-scripts
COPY web/ .
RUN npm run build

FROM golang:1.25-alpine@sha256:8d22e29d960bc50cd025d93d5b7c7d220b1ee9aa7a239b3c8f55a57e987e8d45 AS backend
WORKDIR /app
ARG VERSION
ARG VCS_REF=unknown
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./cmd/shellyctl/dist
RUN V=${VERSION:-$(cat VERSION 2>/dev/null || echo "dev")} && \
    go build -ldflags="-s -w -X main.AppVersion=${V} -X main.CommitHash=${VCS_REF}" -o /shellyadmin ./cmd/shellyctl

FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d
RUN apk add --no-cache ca-certificates tzdata su-exec
RUN addgroup -S shelly && adduser -S -G shelly -h /app shelly
# S14 — initialise /data and /tmp as shelly-owned at build time so a named
# volume inherits the right ownership on first attach. Operators bind-
# mounting a host directory (e.g. Dockhand's /docker/shellyadmin) must
# still chown the host path to the shelly UID — the entrypoint will fall
# back to su-exec if it cannot write as the non-root user.
RUN mkdir -p /data /tmp /app && chown -R shelly:shelly /data /tmp /app
LABEL org.opencontainers.image.title="ShellyAdmin"
LABEL org.opencontainers.image.description="Self-hosted Shelly fleet management for trusted local networks"
LABEL org.opencontainers.image.source="https://github.com/buliwyf42/shellyadmin"
COPY --from=backend /shellyadmin /usr/local/bin/shellyctl
COPY docker/entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
VOLUME ["/data"]
EXPOSE 8080
# Optional MCP listener; only binds when SHELLYADMIN_MCP_TOKEN is set.
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
