# Everything you need day to day. `make up` then `make seed` then `make run`.

.DEFAULT_GOAL := help
export DATABASE_URL ?= postgres://plot:plot@localhost:5432/plotting?sslmode=disable

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

up: ## Start Postgres and MinIO
	docker compose up -d
	@echo "waiting for postgres..."
	@until docker compose exec -T postgres pg_isready -U plot -d plotting >/dev/null 2>&1; do sleep 1; done
	@echo "ready"

down: ## Stop the local stack (keeps data)
	docker compose down

reset: ## Stop and delete all local data
	docker compose down -v

run: ## Run the API against the local stack
	go run ./cmd/api

seed: ## Fill the database with the demo society
	go run ./cmd/seed

reseed: ## Wipe and re-seed
	go run ./cmd/seed -force

build: ## Build the binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o bin/api ./cmd/api

test: ## Run the Go test suite
	go test ./... -race -count=1

smoke: ## Run the schema smoke tests against the local database
	docker compose exec -T postgres psql -U plot -d plotting -v ON_ERROR_STOP=1 -f - < test/schema_smoke.sql

vet: ## Static checks
	go vet ./...

tidy: ## Sync go.mod / go.sum
	go mod tidy

docker: ## Build the production image
	docker build -t plotting-society:local .

.PHONY: help up down reset run seed reseed build test smoke vet tidy docker
