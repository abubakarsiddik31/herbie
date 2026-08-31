.PHONY: up down backend frontend check fe-check

up:
	docker compose -f deploy/docker-compose.yml up -d
down:
	docker compose -f deploy/docker-compose.yml down
backend:
	cd backend && go run ./cmd/server
frontend:
	cd frontend && npm run dev
check:
	cd backend && go fmt ./... && go build ./... && go vet ./... && go test ./...
fe-check:
	cd frontend && npm run check
