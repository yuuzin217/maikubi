# Variables
WAILS=wails
GO=go

.PHONY: help dev build clean tidy test docker-up docker-down docker-logs

help: ## Display this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

dev: ## Run the application in development mode
	$(WAILS) dev

build: ## Build the application for production
	$(WAILS) build

clean: ## Clean build artifacts
	rm -rf build/bin/*
	cd frontend && rm -rf dist

tidy: ## Tidy Go modules
	$(GO) mod tidy

test: ## Run backend tests
	$(GO) test ./backend/...

docker-up: ## Start mock API servers in docker containers
	docker compose up -d --build

docker-down: ## Stop mock API servers
	docker compose down

docker-logs: ## Show logs of mock API servers
	docker compose logs -f
