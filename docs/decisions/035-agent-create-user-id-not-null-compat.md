# Agent 创建接口补齐 user_id 兼容修复

- **日期**：2026-03-04
- **类型**：代码变更
- **状态**：已实施

## 背景

Admin「新建 Agent」调用 `POST /api/agents` 时报 500。  
根因是部分 SQLite 库中 `agent_configs.user_id` 为 `NOT NULL` 且无可用默认值，而创建 SQL 未写入该列。

## 决策

在 Agent 存储层创建语句中显式写入 `user_id`，并补充增量迁移保证历史库缺列时可自动补齐。

## 变更内容

- 更新 `agent/internal/storage/agents.go`
  - `CreateAgentConfig` 的 `INSERT` 增加 `user_id` 列
  - 新建 Agent 时默认写入空字符串 `""`，避免旧库约束报错
- 更新 `agent/internal/storage/db.go`
  - 增量迁移新增：`ALTER TABLE agent_configs ADD COLUMN user_id TEXT NOT NULL DEFAULT ''`
  - 兼容早期缺少该列的历史数据库

## 考虑过的替代方案

- 仅修改表结构默认值：SQLite 对已存在列的默认值变更不稳定，且对现网旧库不可控
- 在 API 层临时兜底：无法覆盖其它调用入口，问题应在存储写入点一次性解决

## 影响

- `POST /api/agents` 在不同历史版本数据库上都可稳定创建
- 不改变现有 API 协议与前端调用方式
- 后续若引入真正的多用户 Agent 归属，可在当前 `user_id` 字段上平滑演进
