APP=shellyctl

.PHONY: build frontend backend

build: frontend backend

frontend:
	cd web && npm ci && npm run build
	rm -rf cmd/shellyctl/dist
	cp -r web/dist cmd/shellyctl/dist

backend:
	go build -ldflags="-s -w" -o bin/$(APP) ./cmd/shellyctl
