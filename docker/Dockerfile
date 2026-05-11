FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

FROM golang:1.25-alpine AS backend
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

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata su-exec
RUN addgroup -S shelly && adduser -S -G shelly -h /app shelly
RUN mkdir -p /data /tmp /app
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
