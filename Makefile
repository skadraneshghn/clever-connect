# CleverConnect VPN Suite Makefile

.PHONY: all install build build-frontend build-backend run-client run-server clean test lint setup-husky

all: install build

# Install all dependencies
install:
	@echo "=== Installing Go backend dependencies ==="
	go mod download
	@echo "=== Installing Client Frontend dependencies ==="
	cd web/client && bun install
	@echo "=== Installing Server Frontend dependencies ==="
	cd web/server && bun install

# Build the entire application
build: build-frontend build-backend

build-frontend:
	@echo "=== Compiling Client Frontend ==="
	cd web/client && bun run build
	@echo "=== Compiling Server Frontend ==="
	cd web/server && bun run build

build-backend:
	@echo "=== Compiling Go backend binary ==="
	go build -o bin/clever-connect main.go
	@echo "=== Compiling Ehco engine binary ==="
	go build -o bin/ehco github.com/Ehco1996/ehco/cmd/ehco
	@echo "Build successful! Binaries located in bin/"

# Run development environments
run-client:
	@echo "=== Starting Client Panel (Development) ==="
	@APP_MODE=client PORT=8080 go run main.go

run-server:
	@echo "=== Starting Server Panel (Development) ==="
	@APP_MODE=server PORT=8081 go run main.go

# Cleanup build artifacts
clean:
	@echo "=== Cleaning up build files ==="
	rm -rf bin/
	rm -rf web/client/dist
	rm -rf web/server/dist
	rm -f data/client.db data/server_fallback.db

# Dynamic tests
test:
	@echo "=== Running Go tests ==="
	go test ./... -v

# Code Quality Audit
lint:
	@echo "=== Executing Go format audits ==="
	go fmt ./...
	go vet ./...

# Setup Husky Git Hooks
setup-husky:
	@echo "=== Installing Husky Hook Managers ==="
	@mkdir -p .husky
	@echo "#!/bin/sh" > .husky/pre-push
	@echo "echo '=== Husky Pre-Push validation checking... ==='" >> .husky/pre-push
	@echo "make lint && make test" >> .husky/pre-push
	@chmod +x .husky/pre-push
	@echo "Husky validation hooks established on pre-push event successfully."
