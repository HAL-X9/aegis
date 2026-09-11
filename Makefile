SHELL := /bin/bash
.DEFAULT_GOAL := help

APP_NAME       := avito-kitchen
BIN_DIR        := bin
CMD_DIR        := ./cmd
MIGRATIONS_DIR := migrations

DB_DSN ?= postgres://avito_kitchen:avito_kitchen@localhost:5432/avito_kitchen?sslmode=disable

DOCKER_COMPOSE := docker compose

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## --- Build & run ------------------------------------------------------------

.PHONY: build
build: ## Build the binary into bin/
	CGO_ENABLED=0 go build -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)

.PHONY: run
run: ## Run the service locally (requires a reachable Postgres)
	go run $(CMD_DIR)

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

## --- Quality ------------------------------------------------------------

.PHONY: fmt
fmt: ## Format code and run go vet
	gofmt -l -w .
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: test
test: ## Run tests with race detector and coverage
	go test ./... -race -cover

## --- Database ------------------------------------------------------------

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" down

.PHONY: migrate-status
migrate-status: ## Show current migration status
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" status

.PHONY: migrate-create
migrate-create: ## Create a new SQL migration (usage: make migrate-create name=add_orders_table)
	goose -dir $(MIGRATIONS_DIR) create $(name) sql

.PHONY: sqlc
sqlc: ## Regenerate typed DB access code from SQL
	sqlc generate

## --- Docker Compose ------------------------------------------------------------

.PHONY: up
up: ## Build and start the full stack
	$(DOCKER_COMPOSE) up -d --build

.PHONY: down
down: ## Stop the stack
	$(DOCKER_COMPOSE) down

.PHONY: logs
logs: ## Follow logs from all services
	$(DOCKER_COMPOSE) logs -f

.PHONY: ps
ps: ## Show running services
	$(DOCKER_COMPOSE) ps