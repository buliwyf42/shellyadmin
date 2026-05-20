APP=shellyctl

.PHONY: build frontend frontend-sync backend dev-frontend dev-backend

build: frontend backend

frontend:
	cd web && npm ci && npm run build
	$(MAKE) frontend-sync

frontend-sync:
	rm -rf cmd/shellyctl/dist
	cp -r web/dist cmd/shellyctl/dist

backend:
	go build -ldflags="-s -w" -o bin/$(APP) ./cmd/shellyctl

dev-frontend:
	cd web && npm run dev

# Local dev server. Login is admin / dev-secret. SHELLYADMIN_PASS (plaintext)
# was removed in v0.2.0, so the hash is generated on the fly from the dev
# password; the encryption key (mandatory since v0.3.0) is a fixed dev-only
# 32-byte key. Both are dev-only — never reuse for a real deployment.
dev-backend:
	SHELLYADMIN_PASS_HASH="$$(printf 'dev-secret' | go run ./cmd/shellyctl hash-password 2>/dev/null)" \
		SHELLYADMIN_SECRET=dev-cookie-secret \
		SHELLYADMIN_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY= \
		DATA_DIR=./data PORT=8080 COOKIE_SECURE=false go run ./cmd/shellyctl
