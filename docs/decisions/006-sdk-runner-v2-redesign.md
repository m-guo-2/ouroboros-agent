# SDK Runner v2 重设计

- **日期**：2026-02-10
- **类型**：架构重构
- **状态**：已实施

## 背景

原 sdk-runner 设计存在多处耦合：systemPrompt 混入了记忆数据、渠道上下文、工具描述等运行时信息；session 依赖 SDK 的 resume 机制，agent 自身无法控制上下文窗口；webui 和 IM 渠道逻辑混合，增加了复杂度。需要一次彻底的重设计，让 agent 更自主、架构更清晰。

## 决策

围绕 8 条原则重写 sdk-runner：

1. **Session = channel:uniqueId** — 群聊用 conversationId，私聊用 userId，统一标识
2. **systemPrompt = agentPrompt + skills** — 仅此两部分，不注入记忆/渠道/工具描述
3. **Append-only** — 消息和 trace 事件只追加不修改
4. **maxTurns = 20** — 不区分渠道，统一上限；每次加载最近 20 条历史消息
5. **压缩是 action** — 由 skills 描述，agent 自主决定何时压缩历史
6. **记忆是 resource** — 长期/短期两种，由 skills 描述，agent 自主读取
7. **仅 IM 渠道** — 移除 webui 场景和 SSE 流式接口
8. **完整可观测性** — 全链路 trace append-only 存储

## 变更内容

- **`agent/src/services/sdk-runner.ts`** — 完全重写：
  - 新增 `resolveSessionKey()` 按 channel:uniqueId 定位 session
  - 移除 SDK resume 机制，每次独立调用 SDK
  - systemPrompt 仅拼接 agentPrompt + skills
  - 历史消息通过 `buildContextPrompt()` 注入 prompt
  - 移除 webui 分支逻辑，始终走 sendToChannel
  - `cleanupInterruptedSessions()` 替代 `resumeInterruptedSessions()`

- **`agent/src/services/context-composer.ts`** — 大幅简化：
  - 仅保留 `buildSystemPrompt()` 和 `buildContextPrompt()` 两个函数
  - 移除 memory 注入、channel context 注入、channel tool prompt

- **`agent/src/services/server-client.ts`** — 新增 API：
  - `findSessionByKey(agentId, sessionKey)` — 按 session key 查找
  - `createSession` 增加 `sessionKey` 字段

- **`agent/src/routes/process.ts`** — 移除 `/process/stream` SSE 端点
- **`agent/src/index.ts`** — 更新导入和启动逻辑

## 考虑过的替代方案

- **保留 SDK resume**：可维持跨调用的工具状态，但失去上下文窗口控制权。IM 场景每条消息相对独立，无需跨调用状态，故选择不 resume。
- **记忆自动注入 prompt**：更简单，但 agent 无法控制上下文使用。改为 resource 模式，agent 按需读取，更符合自主决策的设计哲学。

## 影响

- Server 端需新增 `/api/data/sessions/by-key` 查询接口
- 记忆读写和历史压缩需作为 skills 配置到 agent profile 中
- 原有依赖 SDK resume 的断点续传能力不再可用，改为 append-only 保证数据不丢
- webui 场景需通过其他方式支持（不在 agent 层）
