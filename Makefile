# ─── 配置 ─────────────────────────────────────────────────────────────────
APP_NAME    ?= moli
INSTALL_DIR ?= /opt/$(APP_NAME)
GO_LDFLAGS  := -s -w
MAKEFLAGS   += --no-print-directory

# ─── 源文件发现（用于增量编译依赖） ───────────────────────────────────────
AGENT_SRC  := $(shell find agent/  -name '*.go' -not -path '*/vendor/*')
QIWEI_SRC  := $(shell find channel-qiwei/ -name '*.go' -not -path '*/vendor/*')
ADMIN_SRC  := $(shell find admin/src/ -type f 2>/dev/null) admin/index.html admin/vite.config.ts admin/tsconfig.app.json

# ─── 开发 ─────────────────────────────────────────────────────────────────
.PHONY: all help dev dev-core run-admin run-agent run-feishu run-qiwei

all: help

help:
	@echo ""
	@echo "  开发："
	@echo "    make dev          全部服务 (agent + admin + feishu + qiwei)"
	@echo "    make dev-core     核心服务 (agent + admin)"
	@echo "    make run-admin    仅 Admin 前端 (Vite)"
	@echo "    make run-agent    仅 Agent"
	@echo "    make run-feishu   仅 Feishu Channel"
	@echo "    make run-qiwei    仅 Qiwei Channel"
	@echo ""
	@echo "  编译："
	@echo "    make build        并行编译全部 (admin + agent + qiwei)"
	@echo "    make build-admin  仅 Admin SPA"
	@echo "    make build-agent  仅 Agent 二进制"
	@echo "    make build-qiwei  仅 Qiwei 二进制"
	@echo "    make clean        清理全部产物"
	@echo ""
	@echo "  部署 (supervisor)："
	@echo "    make deploy       编译 + 安装 + 重启"
	@echo "    make install      安装产物到 $(INSTALL_DIR)/"
	@echo "    make restart      重启服务组"
	@echo "    make stop         停止服务组"
	@echo "    make status       查看服务状态"
	@echo ""

dev:
	@trap 'kill 0' EXIT; \
	$(MAKE) run-agent & \
	$(MAKE) run-admin & \
	$(MAKE) run-feishu & \
	$(MAKE) run-qiwei & \
	wait

dev-core:
	@trap 'kill 0' EXIT; \
	$(MAKE) run-agent & \
	$(MAKE) run-admin & \
	wait

run-admin:
	@echo "=> Starting Admin frontend..."
	cd admin && bun run dev

run-agent:
	@echo "=> Starting Go Agent..."
	cd agent && DB_PATH=$(CURDIR)/data/config.db \
	            LOG_DIR=$(CURDIR)/data/logs \
	            ADMIN_DIST=$(CURDIR)/admin/dist \
	            go run ./cmd/agent/main.go

run-feishu:
	@echo "=> Starting Feishu Channel..."
	cd channel-feishu && go run .

run-qiwei:
	@echo "=> Starting Qiwei Channel..."
	cd channel-qiwei && go run .

# ─── 编译（增量 + 并行） ──────────────────────────────────────────────────
.PHONY: build build-admin build-agent build-qiwei clean

build: build-admin build-agent build-qiwei
	@echo "=> 全部编译完成"

# Admin SPA — 依赖源码 + lockfile，文件没变就跳过
admin/dist: $(ADMIN_SRC) admin/bun.lock
	@echo "=> [Admin] bun install + build..."
	cd admin && bun install --frozen-lockfile && bun run build
	@touch admin/dist

build-admin: admin/dist

# Agent 二进制 — 依赖 agent/**/*.go
bin/agent: $(AGENT_SRC) agent/go.mod agent/go.sum
	@echo "=> [Agent] 编译二进制..."
	@mkdir -p bin
	cd agent && go mod tidy && go mod vendor
	cd agent && CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -trimpath -o $(CURDIR)/bin/agent ./cmd/agent

build-agent: bin/agent

# Qiwei 二进制 — 依赖 channel-qiwei/**/*.go
bin/channel-qiwei: $(QIWEI_SRC) channel-qiwei/go.mod channel-qiwei/go.sum
	@echo "=> [Qiwei] 编译二进制..."
	@mkdir -p bin
	cd channel-qiwei && go mod tidy && go mod vendor
	cd channel-qiwei && CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -trimpath -o $(CURDIR)/bin/channel-qiwei .

build-qiwei: bin/channel-qiwei

clean:
	@echo "=> 清理产物..."
	rm -rf bin/ admin/dist

# ─── 部署（supervisor） ───────────────────────────────────────────────────
.PHONY: deploy install install-bins install-admin restart stop status

deploy: build install restart status
	@echo ""
	@echo "=> 部署完成 ✓"

install: install-bins install-admin

install-bins: build-agent build-qiwei
	@sudo mkdir -p $(INSTALL_DIR)/bin
	@for name in agent channel-qiwei; do \
		src="bin/$$name"; \
		dst="$(INSTALL_DIR)/bin/$$name"; \
		if [ -f "$$dst" ]; then \
			bak="$$dst.bak.$$(date +%Y%m%d%H%M%S)"; \
			sudo cp "$$dst" "$$bak"; \
			echo "  备份: $$name → $$(basename $$bak)"; \
		fi; \
		sudo cp "$$src" "$$dst"; \
		sudo chmod 0755 "$$dst"; \
		echo "  安装: $$dst"; \
	done

install-admin: build-admin
	@sudo mkdir -p $(INSTALL_DIR)/admin
	sudo rm -rf $(INSTALL_DIR)/admin/dist
	sudo cp -R admin/dist $(INSTALL_DIR)/admin/dist
	@echo "  安装: $(INSTALL_DIR)/admin/dist/"

restart:
	@echo "=> 重启 $(APP_NAME) 服务组..."
	sudo supervisorctl restart "$(APP_NAME):*"

stop:
	@echo "=> 停止 $(APP_NAME) 服务组..."
	sudo supervisorctl stop "$(APP_NAME):*"

status:
	@sudo supervisorctl status "$(APP_NAME):*" 2>/dev/null || sudo supervisorctl status
