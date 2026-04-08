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

dev-backend:
	SHELLYADMIN_PASS=dev-secret SHELLYADMIN_SECRET=dev-cookie-secret DATA_DIR=./data PORT=8080 COOKIE_SECURE=false go run ./cmd/shellyctl
