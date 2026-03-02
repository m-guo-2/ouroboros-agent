.PHONY: all admin agent feishu qiwei build clean dev dev-core

# Default target
all: help

help:
	@echo "Available commands:"
	@echo "  make dev        - Start all services (agent + admin + feishu)"
	@echo "  make dev-core   - Start core services (agent + admin)"
	@echo "  make admin      - Start the Admin frontend (Vite dev server)"
	@echo "  make agent      - Start the Go Agent (port 1997)"
	@echo "  make feishu     - Start the Feishu Channel service (port 1999)"
	@echo "  make qiwei      - Start the Qiwei Channel service (port 2000)"
	@echo "  make build      - Build admin SPA + Go binary"

# Start all development services in parallel (Ctrl+C stops all)
dev:
	@trap 'kill 0' EXIT; \
	$(MAKE) agent & \
	$(MAKE) admin & \
	$(MAKE) feishu & \
	wait

# Start core services only (agent + admin)
dev-core:
	@trap 'kill 0' EXIT; \
	$(MAKE) agent & \
	$(MAKE) admin & \
	wait

# Start the admin frontend (dev mode)
admin:
	@echo "=> Starting Admin frontend..."
	cd admin && bun run dev

# Start the Go agent
# DB_PATH and LOG_DIR are resolved relative to the repo root, not agent/
agent:
	@echo "=> Starting Go Agent..."
	cd agent && DB_PATH=$(shell pwd)/data/config.db \
	            LOG_DIR=$(shell pwd)/data/logs \
	            ADMIN_DIST=$(shell pwd)/admin/dist \
	            go run ./cmd/agent/main.go

# Build admin SPA then Go binary
build:
	@echo "=> Building Admin SPA..."
	cd admin && bun install && bun run build
	@echo "=> Building Go binary..."
	cd agent && go build -o ../bin/agent ./cmd/agent
	@echo "=> Done. Run: ./bin/agent"

# Start the Feishu channel
feishu:
	@echo "=> Starting Feishu Channel..."
	cd channel-feishu && bun run dev

# Start the Qiwei channel
qiwei:
	@echo "=> Starting Qiwei Channel..."
	cd channel-qiwei && bun run dev

# Clean up all build artifacts
clean:
	@echo "=> Cleaning up..."
	rm -rf admin/dist bin/ channel-feishu/dist channel-qiwei/dist
