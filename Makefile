.PHONY: help build run stop clean test docker-build docker-run docker-stop docker-logs docker-shell compose-up compose-down compose-logs compose-restart embeddings embeddings-dry import test-rag test-rag-stats

# Variables
BINARY_NAME=telegram-llm-bot
DOCKER_IMAGE=telegram-llm-bot
DOCKER_CONTAINER=telegram-llm-bot

# Default target
help:
	@echo "Available commands:"
	@echo "  make build           - Build the Go binary"
	@echo "  make run             - Run the bot locally"
	@echo "  make stop            - Stop the running bot"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make test            - Run tests"
	@echo ""
	@echo "RAG/Embeddings commands:"
	@echo "  make embeddings      - Generate embeddings for all unindexed messages"
	@echo "  make embeddings-dry  - Dry run (show what would be processed)"
	@echo "  make embeddings-batch BATCH=100 - Custom batch size"
	@echo "  make embeddings-limit LIMIT=1000 - Process only N messages"
	@echo "  make import FILE=result.json - Import Telegram export"
	@echo "  make import FILE=result.json DRY_RUN=true - Import dry run"
	@echo ""
	@echo "RAG Testing commands:"
	@echo "  make test-rag-stats  - Show RAG statistics and indexing status"
	@echo "  make test-rag QUERY=\"вопрос\" - Test RAG search"
	@echo "  make test-rag QUERY=\"вопрос\" TOP=5 THRESHOLD=0.7 - Custom params"
	@echo ""
	@echo "Docker commands:"
	@echo "  make docker-build    - Build Docker image"
	@echo "  make docker-run      - Run Docker container"
	@echo "  make docker-stop     - Stop Docker container"
	@echo "  make docker-logs     - View Docker logs"
	@echo "  make docker-shell    - Open shell in container"
	@echo ""
	@echo "Docker Compose commands:"
	@echo "  make compose-up      - Start services with docker compose"
	@echo "  make compose-down    - Stop services with docker compose"
	@echo "  make compose-logs    - View logs from docker compose"
	@echo "  make compose-restart - Restart services"
	@echo "  make compose-rebuild - Rebuild and restart services (no cache)"

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) ./cmd/bot
	@echo "Build complete: $(BINARY_NAME)"

# Run locally
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME)

# Stop the bot (requires process management)
stop:
	@echo "Stopping $(BINARY_NAME)..."
	pkill -f $(BINARY_NAME) || true

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -rf dist/
	go clean
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run || go vet ./...

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE) .
	@echo "Docker image built: $(DOCKER_IMAGE)"

# Docker run
docker-run:
	@echo "Running Docker container..."
	docker run -d \
		--name $(DOCKER_CONTAINER) \
		--env-file .env \
		--restart unless-stopped \
		$(DOCKER_IMAGE)
	@echo "Docker container started: $(DOCKER_CONTAINER)"

# Docker stop
docker-stop:
	@echo "Stopping Docker container..."
	docker stop $(DOCKER_CONTAINER) || true
	docker rm $(DOCKER_CONTAINER) || true
	@echo "Docker container stopped"

# Docker logs
docker-logs:
	@echo "Viewing Docker logs..."
	docker logs -f $(DOCKER_CONTAINER)

# Docker shell
docker-shell:
	@echo "Opening shell in Docker container..."
	docker exec -it $(DOCKER_CONTAINER) /bin/sh

# Docker Compose up
compose-up:
	@echo "Starting services with docker compose..."
	docker compose up -d
	@echo "Services started"

# Docker Compose down
compose-down:
	@echo "Stopping services with docker compose..."
	docker compose down
	@echo "Services stopped"

# Docker Compose logs
compose-logs:
	@echo "Viewing docker compose logs..."
	docker compose logs -f

# Docker Compose restart
compose-restart:
	@echo "Restarting services..."
	docker compose restart
	@echo "Services restarted"

# Docker Compose rebuild and restart
compose-rebuild:
	@echo "🛑 Stopping containers..."
	docker compose down
	@echo "🔨 Building containers (no cache)..."
	docker compose build --no-cache
	@echo "🚀 Starting containers..."
	docker compose up -d
	@echo "✅ Services rebuilt and restarted"
	@echo "📋 To view logs: make compose-logs"

# Quick rebuild alias
rebuild: compose-rebuild

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod verify
	@echo "Dependencies installed"

# Update dependencies
deps-update:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy
	@echo "Dependencies updated"

# Create .env from example
env-setup:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo ".env file created from .env.example"; \
		echo "Please edit .env with your credentials"; \
	else \
		echo ".env file already exists"; \
	fi

# Generate embeddings for all unindexed messages
embeddings:
	@echo "Generating embeddings for all unindexed messages..."
	@go run scripts/generate_embeddings.go

# Generate embeddings (dry run)
embeddings-dry:
	@echo "Running embeddings generation in dry-run mode..."
	@go run scripts/generate_embeddings.go -dry-run

# Generate embeddings with custom batch size
embeddings-batch:
	@echo "Generating embeddings with batch size $(BATCH)..."
	@go run scripts/generate_embeddings.go -batch=$(BATCH)

# Generate embeddings with limit
embeddings-limit:
	@echo "Generating embeddings with limit $(LIMIT)..."
	@go run scripts/generate_embeddings.go -limit=$(LIMIT)

# Import Telegram export JSON
import:
	@if [ -z "$(FILE)" ]; then \
		echo "Usage: make import FILE=path/to/result.json"; \
		echo "   or: make import FILE=path/to/result.json DRY_RUN=true"; \
		exit 1; \
	fi
	@if [ "$(DRY_RUN)" = "true" ]; then \
		echo "Importing (dry-run): $(FILE)..."; \
		go run scripts/import_telegram_export.go -file=$(FILE) -dry-run; \
	else \
		echo "Importing: $(FILE)..."; \
		go run scripts/import_telegram_export.go -file=$(FILE); \
	fi

# Test RAG search
test-rag:
	@if [ -z "$(QUERY)" ]; then \
		echo "Usage: make test-rag QUERY=\"ваш вопрос\""; \
		echo "   or: make test-rag QUERY=\"вопрос\" TOP=5 THRESHOLD=0.7"; \
		echo ""; \
		echo "Examples:"; \
		echo "  make test-rag QUERY=\"Что говорили про игры?\""; \
		echo "  make test-rag QUERY=\"крипта\" THRESHOLD=0.5"; \
		go run scripts/test_rag.go; \
	else \
		go run scripts/test_rag.go -query="$(QUERY)" -top=$(or $(TOP),5) -threshold=$(or $(THRESHOLD),0.7); \
	fi

# Show RAG statistics
test-rag-stats:
	@echo "RAG System Statistics:"
	@go run scripts/test_rag.go
