.PHONY: all admin server agent feishu qiwei clean

# Default target
all: help

help:
	@echo "Available commands:"
	@echo "  make admin      - Start the Admin frontend (Vite)"
	@echo "  make server     - Start the main Server (Bun)"
	@echo "  make agent      - Start the Go Agent"
	@echo "  make feishu     - Start the Feishu Channel service (Bun)"
	@echo "  make qiwei      - Start the Qiwei Channel service (Bun)"
	@echo "  make start-all  - Start Server, Agent, and Admin together (requires tmux or separate terminals)"

# Start the admin frontend
admin:
	@echo "=> Starting Admin frontend..."
	cd admin && bun run dev

# Start the main server
server:
	@echo "=> Starting Main Server..."
	cd server && bun run dev

# Start the Go agent
agent:
	@echo "=> Starting Go Agent..."
	cd agent && go run ./cmd/agent/main.go

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
	cd agent && make clean || true
	rm -rf admin/dist server/dist channel-feishu/dist channel-qiwei/dist
