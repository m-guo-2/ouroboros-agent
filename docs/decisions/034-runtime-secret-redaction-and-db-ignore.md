# 运行时密钥脱敏与 SQLite 落库防泄漏

- **日期**：2026-03-04
- **类型**：代码变更
- **状态**：已实施

## 背景

GitHub Push Protection 拦截了推送，原因是 `agent/data/config.db` 历史提交中包含 Figma Personal Access Token。问题根因是运行时数据库被纳入版本管理，且工具调用输入/输出在日志与上下文持久化路径中未做统一脱敏。

## 决策

在 Agent 侧引入统一字符串脱敏函数，覆盖工具日志、结构化消息落库、会话上下文持久化三条路径，同时把 `agent/data/config.db*` 明确加入忽略规则并停止追踪该数据库文件。

## 变更内容

- 新增 `agent/internal/sanitize/redact.go`：
  - 对 `figd_...` 模式（Figma PAT）做强匹配脱敏。
  - 对 `api_key/token/secret/access_token` 常见赋值片段做通用脱敏。
- 修改 `agent/internal/engine/loop.go`：
  - `toolInput`/`toolResult` 写入业务日志前统一脱敏，避免日志层泄漏。
- 修改 `agent/internal/runner/processor.go`：
  - `toPersistableMessages` 在写入 `messages` 表前统一脱敏。
  - 新增 `redactMessagesForStorage`，在更新 `agent_sessions.context` 前统一脱敏。
- 修改 `.gitignore`：
  - 增加 `agent/data/config.db`、`agent/data/config.db-shm`、`agent/data/config.db-wal`。
- 执行 `git rm --cached agent/data/config.db`，停止跟踪运行时数据库文件。

## 考虑过的替代方案

- 仅在 `.gitignore` 中忽略数据库：不能解决日志与上下文继续写入敏感串的问题。
- 仅在单个工具（如 shell）输出层脱敏：覆盖范围不足，仍会在其他路径泄漏。

## 影响

后续工具执行结果即使包含 token 形态，也会在可持久化路径中被替换为 `[REDACTED]`。这会轻微降低历史上下文的原始细节，但可显著降低敏感信息落库和误提交风险。
