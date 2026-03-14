# Subagent 异步工具：自然语言聚合优先

- **日期**：2026-02-28
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

当前 Agent 已具备 ReAct 循环、工具调用、上下文压缩和分层日志能力，但缺少“异步子任务”机制。复杂请求下，主 Agent 需要并行委托多个子任务，再聚合结果给用户。

本次设计强调两点：

- 避免过度设计，不引入复杂协议层；
- 子任务结果以自然语言回传，便于主 Agent 直接整合。

## 决策

将 subagent 设计为内建异步工具，采用“启动 + 查询 + 完成主动通知”接口：

1. `run_subagent_async`：启动异步 subagent 任务，立即返回 `jobId`；
2. `get_subagent_status`：查询任务状态与最终自然语言总结；
3. 主 Agent 可在需要时轮询并聚合多个 subagent 的自然语言结果，不强制结构化 output schema；
4. subagent 完成后会主动向主 Agent 会话注入一条内部完成通知（push），触发主 Agent 继续处理；
5. 默认提供两个 subagent 类型：`developer` 与 `file_analysis`，均使用内置默认 system prompt。

subagent 默认复用主 Agent 的压缩上下文（同一轮历史消息），保证语义连续性，不新增复杂上下文传递机制。  
subagent 的 system prompt 不继承主 Agent prompt，按子代理类型使用固定内置模板。

## 变更内容

- 新增 `agent/internal/subagent/manager.go`：
  - 维护异步 subagent 任务生命周期（queued/running/completed/failed）；
  - 在后台运行独立 Agent Loop；
  - 产出自然语言总结并保存任务元信息；
  - 将任务细节落盘到 `data/subagents/<jobId>/`。
- 在 `agent/internal/runner/processor.go` 注册两个内建工具：
  - `run_subagent_async`
  - `get_subagent_status`
- subagent 执行时过滤会造成递归与副作用扩散的工具（如再次创建 subagent、直接向用户发消息）。

## 考虑过的替代方案

- 固定事件 schema + 固定 output schema：
  - 优点：自动化消费更稳；
  - 否决原因：当前阶段复杂度过高，且限制主 Agent 使用自然语言灵活聚合。
- 单次同步 subagent 调用：
  - 优点：实现简单；
  - 否决原因：阻塞主循环，不适合长任务和多子任务并行场景。

## 影响

- 主 Agent 可并行派发多个子任务，并以自然语言汇总结果；
- 子任务细节不进入主上下文，减少噪声和 token 压力；
- 后续若需要自动化编排，可在当前最小模型上逐步增加结构化协议，而不影响现有流程。
