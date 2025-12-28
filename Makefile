.PHONY: all build run test lint clean docker-build docker-up docker-down migrate

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOLINT=golangci-lint

# Binary names
BOT_BINARY=bin/bot
WORKER_BINARY=bin/worker

# Build flags
LDFLAGS=-ldflags "-w -s"

all: lint test build

## Build commands
build: build-bot build-worker

build-bot:
	@echo "Building bot..."
	$(GOBUILD) $(LDFLAGS) -o $(BOT_BINARY) ./cmd/bot

build-worker:
	@echo "Building worker..."
	$(GOBUILD) $(LDFLAGS) -o $(WORKER_BINARY) ./cmd/worker

## Run commands
run-bot: build-bot
	@echo "Running bot..."
	./$(BOT_BINARY)

run-worker: build-worker
	@echo "Running worker..."
	./$(WORKER_BINARY)

## Test commands
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## Lint commands
lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

## Clean commands
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

## Docker commands
docker-build:
	@echo "Building Docker images..."
	docker-compose build

docker-up:
	@echo "Starting Docker containers..."
	docker-compose up -d

docker-down:
	@echo "Stopping Docker containers..."
	docker-compose down

docker-logs:
	docker-compose logs -f

## Database commands
migrate-up:
	@echo "Running migrations..."
	$(GOCMD) run ./cmd/migrate up

migrate-down:
	@echo "Rolling back migrations..."
	$(GOCMD) run ./cmd/migrate down

migrate-create:
	@echo "Creating new migration..."
	@read -p "Migration name: " name; \
	touch migrations/$$(date +%Y%m%d%H%M%S)_$$name.up.sql; \
	touch migrations/$$(date +%Y%m%d%H%M%S)_$$name.down.sql

## Development helpers
dev-setup:
	@echo "Setting up development environment..."
	$(GOMOD) download
	$(GOMOD) tidy

generate:
	@echo "Running go generate..."
	$(GOCMD) generate ./...

## Fly.io deployment
deploy-bot:
	@echo "Deploying bot to Fly.io..."
	fly deploy --config fly.bot.toml

deploy-worker:
	@echo "Deploying worker to Fly.io..."
	fly deploy --config fly.worker.toml

## Help
help:
	@echo "Alem Community Hub - Available commands:"
	@echo ""
	@echo "  make build          - Build all binaries"
	@echo "  make run-bot        - Run the Telegram bot"
	@echo "  make run-worker     - Run the background worker"
	@echo "  make test           - Run tests"
	@echo "  make lint           - Run linter"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make docker-up      - Start Docker containers"
	@echo "  make docker-down    - Stop Docker containers"
	@echo "  make migrate-up     - Run database migrations"
	@echo "  make deploy-bot     - Deploy bot to Fly.io"
