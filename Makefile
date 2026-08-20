.PHONY: build run test bench lint fmt vet tidy up down logs clean help

BINARY      := app
CMD_PATH    := ./cmd
CONFIG      := configs/aegis.yaml
ROUTES      := configs/gateway.yaml

build: ## Build the gateway binary
	go build -o $(BINARY) $(CMD_PATH)

run: ## Run the gateway from source
	go run $(CMD_PATH) -config $(CONFIG) -routes $(ROUTES)

mock: ## Run the local mock upstream
	cd mock && go run server.go

test: ## Run all unit and integration tests
	go test ./...

fmt: ## Format all Go source
	gofmt -l -w .

vet: ## Run go vet
	go vet ./...

lint: fmt vet ## Format + vet (add golangci-lint here if/when adopted)

up: ## Start the full stack via docker compose
	docker compose up -d --build

down: ## Stop the docker compose stack
	docker compose down

logs: ## Follow docker compose logs
	docker compose logs -f
