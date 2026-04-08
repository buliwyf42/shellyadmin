FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

FROM golang:1.23-alpine AS backend
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./cmd/shellyctl/dist
RUN go build -ldflags="-s -w" -o /shellyctl ./cmd/shellyctl

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S shelly && adduser -S -G shelly -h /app shelly
RUN mkdir -p /data /tmp && chown -R shelly:shelly /data /tmp /app
COPY --from=backend /shellyctl /usr/local/bin/shellyctl
USER shelly
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/shellyctl"]
