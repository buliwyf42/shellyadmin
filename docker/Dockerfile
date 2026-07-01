FROM node:26-alpine@sha256:144769ec3f32e8ee36b3cfde91e82bee25d9367b20f31a151f3f7eea3a2a8541 AS frontend
WORKDIR /app/web
COPY web/package*.json ./
# --ignore-scripts blocks malicious postinstall hooks in compromised npm deps;
# Svelte/Vite/TS don't require lifecycle scripts at install time. If a future
# dep needs a build hook, allowlist it via `npm rebuild <pkg>` explicitly.
RUN npm ci --ignore-scripts
COPY web/ .
RUN npm run build

FROM golang:1.26-alpine@sha256:f23e8b227fb4493eabe03bede4d5a32d04092da71962f1fb79b5f7d1e6c2a17f AS backend
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

FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40
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
