.PHONY: build web server dev test
build: web server
web:
	cd web && npm ci && npm run build
server:
	CGO_ENABLED=0 go build -o 24cups ./cmd/24cups
test:
	go test ./...
	cd web && npm run typecheck
# Run `go run ./cmd/24cups` and `cd web && npm run dev` in two terminals;
# Vite (http://localhost:5173) proxies /api to the Go server on :8080.
# Rate limits are off in dev so seeding and testing from one IP works.
# DAY=3 make dev pretends it's December 3 (server and frontend).
dev:
	go run ./cmd/24cups -no-rate-limits $(if $(DAY),-dev-day=$(DAY))
