.PHONY: all setup dev dev-infra dev-api dev-worker dev-web test test-unit test-integration test-e2e test-isolation lint format security build migrate-up migrate-down reset-db clean generate-api-types generate-ent

# Default target
all: test build

# ------------------------------------------------------------------------------
# Setup & Local Development
# ------------------------------------------------------------------------------
setup:
	@echo "Setting up ContextForge environment..."
	@if [ ! -f .env ]; then cp .env.example .env && echo "Created .env from .env.example"; fi
	@cd apps/api && go mod download
	@cd apps/web && npm install

# Starts local infrastructure (Postgres + Redis) in Docker
dev-infra:
	docker compose up -d postgres redis

# Runs the Go REST API on the host machine (accessing host CLI tools like opencode)
dev-api:
	cd apps/api && go run cmd/api/main.go

# Runs the background ingestion worker on the host machine
dev-worker:
	cd apps/api && go run cmd/worker/main.go

# Runs the Next.js web application on the host machine
dev-web:
	cd apps/web && npm run dev

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
	cd apps/api && go test -v -race -cover ./pkg/...

test-integration:
	cd apps/api && go test -v -race -tags=integration ./tests/integration/...

test-isolation:
	cd apps/api && go test -v -race -run TestCrossProjectIsolation ./tests/integration/...

test-e2e:
	cd apps/web && npm run test:e2e

# ------------------------------------------------------------------------------
# Code Quality & Security
# ------------------------------------------------------------------------------
lint:
	cd apps/api && golangci-lint run ./...
	cd apps/web && npm run lint

format:
	cd apps/api && gofmt -s -w .
	cd apps/web && npm run format

security:
	cd apps/api && govulncheck ./...
	cd apps/api && gosec -quiet ./...
	cd apps/web && npm audit

# ------------------------------------------------------------------------------
# Build & Code Generation
# ------------------------------------------------------------------------------
build:
	cd apps/api && go build -o ../../bin/api cmd/api/main.go
	cd apps/api && go build -o ../../bin/worker cmd/worker/main.go
	cd apps/web && npm run build

generate-ent:
	cd apps/api && go generate ./ent

generate-api-types:
	npx openapi-typescript docs/api/openapi.yaml -o apps/web/src/types/api.ts

# ------------------------------------------------------------------------------
# Database Migrations
# ------------------------------------------------------------------------------
migrate-up:
	cd apps/api && go run cmd/migrate/main.go up

migrate-down:
	cd apps/api && go run cmd/migrate/main.go down

reset-db:
	cd apps/api && go run cmd/migrate/main.go reset

# ------------------------------------------------------------------------------
# Cleanup
# ------------------------------------------------------------------------------
clean:
	rm -rf bin/ dist/ tmp/ coverage.out coverage.html apps/web/.next
	docker compose down -v
