# Subagent 默认角色与内置系统提示词

- **日期**：2026-02-28
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

在 subagent 异步机制落地后，需要进一步收敛使用方式，避免主 Agent 每次动态拼装子代理角色和系统提示词，降低复杂度和不确定性。

## 决策

1. subagent 固定为两个默认角色：
   - `developer`
   - `file_analysis`
2. 两个角色均使用内置默认 system prompt。
3. subagent 不继承主 Agent 的 system prompt，仅复用同轮上下文消息。
4. subagent 任务完成（成功/失败）后，主动向主 Agent 所在 session 推送一条内部完成通知消息。
5. 两个默认 subagent 使用最小工具白名单：仅暴露各自任务所需工具。

## 变更内容

- `agent/internal/subagent/manager.go`
  - 新增 `Profile` 字段；
  - 增加 profile 归一化与校验；
  - 内置两套默认提示词模板；
  - 启动 subagent 时按 profile 选择 system prompt；
  - 按 profile 过滤工具为最小白名单。
- `agent/internal/runner/processor.go`
  - `run_subagent_async` 工具参数由 `name` 调整为 `subagent`；
  - 返回值中增加 `subagent` 字段；
  - 调用 manager 时不再传入主 Agent system prompt；
  - 为 subagent 注册完成回调，自动将完成通知入队到主 Agent 会话（push）。

## 影响

- 调用 subagent 的方式更稳定、简单；
- 子代理角色边界更清晰；
- 主 Agent 可在无轮询情况下自动收到 subagent 完成信号并继续执行；
- 子代理工具权限更小，误操作与副作用风险进一步降低；
- 后续若扩展新子代理类型，可按同样方式添加 profile 与默认提示词，不影响现有调用。
