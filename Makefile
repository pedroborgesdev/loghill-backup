.PHONY: dev backend frontend build test lint docker
backend:
	go run ./cmd/server
frontend:
	cd frontend && npm run dev
dev:
	@echo "Execute 'make backend' e 'make frontend' em terminais separados"
build:
	cd frontend && npm ci && npm run build
	go build -o logmate ./cmd/server
test:
	go test -race ./...
	cd frontend && npm run test:run
lint:
	go vet ./...
	cd frontend && npm run lint
docker:
	docker compose up --build
