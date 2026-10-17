SHELL := /bin/bash
.DEFAULT_GOAL := help

APP_NAME := aegis
BIN_DIR := bin
CMD_DIR := ./cmd
DOCKER_COMPOSE := docker compose

.PHONY: help
help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the Aegis binary
	CGO_ENABLED=0 go build -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)

.PHONY: run
run: ## Run Aegis locally
	go run $(CMD_DIR) \
		-config configs/aegis.yaml \
		-routes configs/gateway.yaml

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

.PHONY: fmt
fmt: ## Format Go source files
	gofmt -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: test
test: ## Run tests
	go test ./...

.PHONY: test-race
test-race: ## Run tests with race detector
	go test ./... -race

# Every fuzz target as <package>:<target>. `go test -fuzz` accepts one target in
# one package per invocation, so the fuzz recipe loops over this list.
FUZZ_TARGETS := \
	./internal/dataplane/request:FuzzCanonicalPath \
	./internal/dataplane/proxy:FuzzExecutorPathAgreement \
	./internal/controlplane/validate:FuzzValidatePathPrefix \
	./internal/controlplane/pipeline:FuzzPathPrefixBuild \
	./internal/controlplane/compile:FuzzIndexLookup
FUZZTIME ?= 30s

.PHONY: fuzz
fuzz: ## Fuzz every target for FUZZTIME each (default 30s)
	@for target in $(FUZZ_TARGETS); do \
		pkg=$${target%%:*}; name=$${target##*:}; \
		echo "==> $$name ($(FUZZTIME))"; \
		go test "$$pkg" -run '^$$' -fuzz "^$$name$$" -fuzztime $(FUZZTIME) || exit 1; \
	done

.PHONY: coverage
coverage: ## Run tests with coverage
	go test ./... -cover

.PHONY: bench
bench: ## Run router benchmarks
	go test ./internal/dataplane/router/ -bench . -benchmem

.PHONY: bench-proxy
bench-proxy: ## Run proxy benchmarks (10 repetitions)
	go test ./internal/dataplane/proxy/ -run '^$$' \
		-bench 'BenchmarkProxyServeHTTP$$|BenchmarkRoundTripWarm|BenchmarkFullProxyE2E' \
		-benchmem -count 10

.PHONY: up
up: ## Build and start Docker stack
	$(DOCKER_COMPOSE) up -d --build

.PHONY: down
down: ## Stop Docker stack
	$(DOCKER_COMPOSE) down

.PHONY: logs
logs: ## Follow Docker logs
	$(DOCKER_COMPOSE) logs -f

.PHONY: ps
ps: ## Show Docker services
	$(DOCKER_COMPOSE) ps