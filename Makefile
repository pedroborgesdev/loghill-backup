ifeq ($(OS),Windows_NT)
BINARY := logmate.exe
RUN_BINARY := .\logmate.exe
CLEAR_TERMINAL := cls
else
BINARY := logmate
RUN_BINARY := ./logmate
CLEAR_TERMINAL := clear
endif

.PHONY: backend frontend build run lint docker
backend:
	cd apps/backend && go run ./cmd/server
frontend:
	cd apps/frontend && npm run dev
build:
	cd apps/frontend && npm ci && npm run build
	cd apps/backend && go build -o ../../$(BINARY) ./cmd/server
run: build
	$(CLEAR_TERMINAL)
	$(RUN_BINARY)
lint:
	cd apps/backend && go vet ./...
	cd apps/frontend && npm run lint
docker:
	docker compose up --build
