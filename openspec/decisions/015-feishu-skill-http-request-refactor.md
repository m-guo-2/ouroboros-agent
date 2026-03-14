# 飞书 Skill 重构：http_request + SKILL.md

- **日期**：2026-02-25
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

原先 feishu-agent 为每个 channel-feishu action 定义独立 tool（feishu_send_text、feishu_get_chat_info 等），feishu-operator 为空壳（tools: []）。每次 channel-feishu 新增 action 都需修改 skill.json。用户希望更灵活：单一通用工具，文档承载所有调用说明。

## 决策

采用「单工具 + 文档驱动」设计：

1. **工具**：单一 `http_request`，executor 类型 `shell`，从 input 读取 `command`（完整 curl 命令）并由 sh 执行
2. **文档**：`SKILL.md`（标准 skill 格式）承载 baseUrl、路径、所有 action 及 params
3. **放置位置**：`server/data/skills/feishu-agent/SKILL.md`

## 变更内容

- `server/data/skills/feishu-agent/skill.json`：改为 1 个 tool `http_request`，executor `type: "shell"`
- 新增 `server/data/skills/feishu-agent/SKILL.md`：YAML frontmatter + 完整 action/params 文档
- 删除 `server/data/skills/feishu-agent/README.md`
- 删除 `server/data/skills/feishu-operator/` 目录
- `agent/internal/engine/registry.go`：新增 `createShellExecutor()`，支持 `shell` 类型（执行 input.command 即 curl）
- `server/scripts/migrate-skills-to-db.ts`：优先读取 SKILL.md 作为 readme，支持 `--sync` 覆盖并删除已移除 skill
- 类型扩展：database、admin 增加 `shell` executor 类型

## 影响

- 新增飞书能力时只需更新 SKILL.md，无需改 skill.json
- 模型根据 SKILL 文档构造完整 curl 命令，由 shell executor 直接执行，无需新写 HTTP 逻辑
- 不维护 `.cursor/skills/` 下的 feishu skill
