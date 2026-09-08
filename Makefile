.PHONY: all setup dev dev-infra dev-api dev-worker dev-web test test-unit test-integration test-e2e test-isolation lint format security build migrate-up migrate-down reset-db clean generate-api-types generate-ent

# Default target
all: test build

# ------------------------------------------------------------------------------
# Setup & Local Development
# ------------------------------------------------------------------------------
setup:
	@echo "Setting up ContextForge environment..."
	@if [ ! -f .env ]; then cp .env.example .env && echo "Created .env from .env.example"; fi
	@go mod download
	@cd web && npm install

# Starts local infrastructure (Postgres + Redis) in Docker
dev-infra:
	docker compose -f docker-compose.dev.yml up -d postgres redis

# Runs the Go REST API on the host machine (accessing host CLI tools like opencode)
dev-api:
	go run cmd/api/main.go

# Runs the background ingestion worker on the host machine
dev-worker:
	go run cmd/worker/main.go

# Runs the Next.js web application on the host machine
dev-web:
	cd web && npm run dev

# Full local development orchestration (Host-CLI bridge profile)
dev:
	@echo "Starting Postgres and Redis in background..."
	@$(MAKE) dev-infra
	@echo "Infrastructure ready. In separate terminals, run:"
	@echo "  make dev-api      (Starts Go API server on port 8080)"
	@echo "  make dev-worker   (Starts background ingestion worker)"
	@echo "  make dev-web      (Starts Next.js frontend on port 3000)"

# Runs the full containerized stack via Docker Compose
dev-docker:
	docker compose up --build

# ------------------------------------------------------------------------------
# Testing
# ------------------------------------------------------------------------------
test: test-unit test-integration

test-unit:
	go test -v -race ./internal/...

test-integration:
	go test -v -race ./tests/integration/...

test-isolation:
	go test -v -race -run TestVectorDataIsolation ./tests/integration/...

test-e2e:
	go test -v ./tests/e2e/...

# ------------------------------------------------------------------------------
# Code Quality & Security
# ------------------------------------------------------------------------------
lint:
	go vet ./...
	cd web && npm run lint

format:
	gofmt -s -w .

security:
	go vet ./...
	cd web && npm audit

# ------------------------------------------------------------------------------
# Build & Code Generation
# ------------------------------------------------------------------------------
build:
	go build -o bin/api cmd/api/main.go
	go build -o bin/worker cmd/worker/main.go
	go build -o bin/migrate cmd/migrate/main.go
	cd web && npm run build

generate-ent:
	go generate ./internal/ent

generate-api-types:
	npx openapi-typescript docs/api/openapi.yaml -o web/src/types/api.ts

# ------------------------------------------------------------------------------
# Database Migrations
# ------------------------------------------------------------------------------
migrate-up:
	go run cmd/migrate/main.go up

migrate-down:
	go run cmd/migrate/main.go down

reset-db:
	go run cmd/migrate/main.go reset

# ------------------------------------------------------------------------------
# Cleanup
# ------------------------------------------------------------------------------
clean:
	rm -rf bin/ dist/ tmp/ coverage.out coverage.html web/.next
	docker compose down -v
