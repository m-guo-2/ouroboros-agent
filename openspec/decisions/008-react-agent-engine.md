# ReAct Agent 引擎：从 SDK 黑盒到自定义 while 循环

- **日期**：2026-02-24
- **类型**：架构重构
- **状态**：已实施

## 背景

项目原先使用 `@anthropic-ai/claude-agent-sdk` 作为 Agent 执行引擎。该 SDK 将 LLM 调用、工具执行、会话管理全部封装在 `query()` 方法内部，以子进程方式运行。主要痛点：

1. **不可观测**：无法精确捕获 Agent 的每一步推理（Thought）和工具调用决策（Action），只能从流式事件中间接提取。
2. **不可控**：无法在工具调用前/后插入自定义逻辑（如审计、限流、安全校验）。
3. **架构耦合**：需要维护 API Proxy（Anthropic ↔ OpenAI 格式转换）、SDK 子进程管理、LiveSession 等大量胶水代码。
4. **调试困难**：SDK 子进程的内部状态不透明，错误堆栈经常丢失上下文。

## 决策

抛弃 Claude Agent SDK，自己实现基于 ReAct（Reasoning + Acting）模式的 Agent 执行循环。核心是一个纯手写的 `while` 循环，直接调用 LLM 原生 API。

## 变更内容

新增 `agent/src/engine/` 目录，包含 6 个模块：

- **`types.ts`** — 核心类型定义（AgentMessage, ToolDefinition, LLMClient 等），遵循 Anthropic Messages API 原生格式
- **`loop.ts`** — ReAct 核心循环。一个 `while` 循环：调用 LLM → 解析 tool_use → 执行工具 → 追加 tool_result → 继续循环
- **`tool-registry.ts`** — 统一工具注册中心，聚合三类来源：
  - Builtin（send_channel_message 等内置工具）
  - Skill（从 skill-manager 加载，支持 HTTP / internal executor）
  - MCP（通过 HTTP 从远端 MCP Server 拉取工具列表并代理执行）
- **`llm-client.ts`** — 两种 LLM 客户端实现：
  - `AnthropicClient`：直接调用 Anthropic Messages API（零格式转换）
  - `OpenAICompatibleClient`：调用 OpenAI 兼容 API，内置格式转换
- **`sandbox.ts`** — 本地目录沙盒：路径校验（防逃逸）、受限 Shell 执行
- **`runner.ts`** — 集成层，替代原 `sdk-runner.ts`，负责：
  - 从 ServerClient 加载配置（Agent 配置、Skills、模型凭据）
  - 构建 ToolRegistry 和 LLMClient
  - 运行 Agent Loop，通过 onEvent 回调上报 Trace
  - Session 生命周期管理（队列串行化、空闲驱逐）

**入口变更**：
- `agent/src/index.ts`：移除 API Proxy 挂载和 `setAgentAppPort` 调用
- `agent/src/routes/process.ts`：import 从 `sdk-runner` 改为 `engine/runner`

## 考虑过的替代方案

1. **LangGraph.js**：提供现成的图状态机和可观测性支持，但引入重依赖，且对 Anthropic tool_use 格式的支持不够原生。
2. **Vercel AI SDK Core**：流式支持好，但工具管理和会话管理不够灵活，且与现有 Skill 系统的集成需要大量适配。
3. **继续用 SDK + 增强日志**：通过 hooks 和流式事件提取来改善可观测性。实际尝试过（sdk-runner.ts 中有大量 stream event 解析代码），效果不理想，代码复杂度极高。

## 影响

- **旧代码保留**：`agent/src/services/sdk-runner.ts` 和 `api-proxy.ts` 暂时保留，不删除，作为回退方案。
- **依赖变化**：不再需要 `@anthropic-ai/claude-agent-sdk`，但当前不急于从 package.json 移除。
- **MCP 接入**：ToolRegistry 已预留 `registerMcpServer()` 方法，通过 HTTP 协议拉取远端工具列表。
- **上下文压缩**：本次不实施。当前每次请求仅发送当前消息（无历史累积），后续可在 runner.ts 中增加历史消息加载和压缩逻辑。
- **Trace 系统完全复用**：onEvent 回调的格式与现有 `execution-trace.ts` 完全兼容，前端 MonitorView 无需改动。
