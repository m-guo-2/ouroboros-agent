## Why

Agent 当前对所有任务一视同仁——收到消息立刻执行。但微信操作（发消息、群管理）不可逆，复杂任务（批量操作、跨群协调）一旦执行错误，无法撤回。需要一种机制让 agent 在面对复杂或高风险任务时，先制定计划并获得用户确认，再动手执行。

借鉴 Claude Code 的 plan mode 设计：本质是 prompt injection 驱动的行为模式切换——同一个 LLM、同一个 loop、同一套工具，通过注入不同指令让模型"先想后做"。

## What Changes

- 新增 `SessionMode` 字段（`normal` / `plan`），持久化在 session context 中
- 新增 `enter_plan_mode` builtin tool，LLM 自主判断任务复杂度后调用进入计划模式
- 新增 `exit_plan_mode` builtin tool，LLM 完成计划后调用，将计划发给用户并退出计划模式
- `processSession` 在 plan 模式下向 messages 注入只读约束指令，引导模型只收集信息、不执行操作
- 用户在微信中回复确认/修改意见后，下一轮 `processSession` 自然将用户消息带入，LLM 根据上下文决定执行或修订

## Capabilities

### New Capabilities
- `plan-mode-state`: session 级别的模式状态管理（normal/plan 切换与持久化）
- `plan-mode-tools`: enter/exit plan mode 两个 builtin tool 的注册与执行
- `plan-mode-prompt-injection`: plan 模式下的只读约束指令注入

### Modified Capabilities

（无现有 spec 需要修改）

## Impact

- `agent/internal/runner/worker.go` — SessionWorker 增加 Mode 字段
- `agent/internal/runner/processor.go` — processSession 中增加模式检测与指令注入逻辑，注册新工具
- `agent/internal/engine/loop.go` — 无需改动（行为通过 prompt 控制，不改 loop）
- `agent/internal/types/` — 新增 SessionMode 类型定义
