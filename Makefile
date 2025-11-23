.PHONY: build run test clean docker lint fmt help

# Variables
BINARY_NAME=graphpost
MAIN_PATH=./cmd/graphpost
VERSION?=1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# Default target
all: build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)

## build-linux: Build for Linux
build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

## build-darwin: Build for macOS
build-darwin:
	@echo "Building $(BINARY_NAME) for macOS..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)

## build-windows: Build for Windows
build-windows:
	@echo "Building $(BINARY_NAME) for Windows..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

## build-all: Build for all platforms
build-all: build-linux build-darwin build-windows

## run: Run the application
run: build
	./$(BINARY_NAME)

## run-dev: Run with development settings
run-dev:
	go run $(MAIN_PATH) --enable-playground --enable-console

## test: Run tests
test:
	go test -v ./...

## test-coverage: Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## lint: Run linter
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed"; \
	fi

## fmt: Format code
fmt:
	go fmt ./...
	goimports -w .

## tidy: Tidy dependencies
tidy:
	go mod tidy

## clean: Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	rm -f coverage.out coverage.html

## docker-build: Build Docker image
docker-build:
	docker build -t graphpost:$(VERSION) .
	docker tag graphpost:$(VERSION) graphpost:latest

## docker-run: Run Docker container
docker-run:
	docker run -it --rm \
		-e GRAPHPOST_DATABASE_URL=$(DATABASE_URL) \
		-p 8080:8080 \
		graphpost:latest

## docker-push: Push Docker image
docker-push:
	docker push graphpost:$(VERSION)
	docker push graphpost:latest

## install: Install the binary
install: build
	cp $(BINARY_NAME) /usr/local/bin/

## deps: Download dependencies
deps:
	go mod download

## generate: Generate code (if needed)
generate:
	go generate ./...

## help: Show this help
help:
	@echo "GraphPost - Instant GraphQL API for PostgreSQL"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
