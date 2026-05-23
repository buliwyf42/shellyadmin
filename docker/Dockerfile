FROM node:26-alpine@sha256:e71ac5e964b9201072425d59d2e876359efa25dc96bb1768cb73295728d6e4ea AS frontend
WORKDIR /app/web
COPY web/package*.json ./
# --ignore-scripts blocks malicious postinstall hooks in compromised npm deps;
# Svelte/Vite/TS don't require lifecycle scripts at install time. If a future
# dep needs a build hook, allowlist it via `npm rebuild <pkg>` explicitly.
RUN npm ci --ignore-scripts
COPY web/ .
RUN npm run build

FROM golang:1.26-alpine@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS backend
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

FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
RUN apk add --no-cache ca-certificates tzdata su-exec
RUN addgroup -S shelly && adduser -S -G shelly -h /app shelly
# S14 — initialise /data and /tmp as shelly-owned at build time so a named
# volume inherits the right ownership on first attach. Operators bind-
# mounting a host directory must still chown the host path to the shelly
# UID — the entrypoint will fall
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
