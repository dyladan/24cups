.PHONY: build web server dev test
build: web server
web:
	cd web && npm ci && npm run build
server:
	CGO_ENABLED=0 go build -o advent-cup ./cmd/advent-cup
test:
	go test ./...
	cd web && npm run typecheck
# Run `go run ./cmd/advent-cup` and `cd web && npm run dev` in two terminals;
# Vite (http://localhost:5173) proxies /api to the Go server on :8080.
dev:
	go run ./cmd/advent-cup
