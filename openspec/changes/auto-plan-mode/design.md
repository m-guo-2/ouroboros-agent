## Context

Agent 当前对所有任务统一执行：收到消息 → `processSession` → `RunAgentLoop` → LLM 调用工具 → 返回结果。没有"先想后做"的机制。

微信操作不可逆（发消息、群管理），复杂任务需要在执行前对齐方案。

借鉴 Claude Code plan mode 的核心洞察：**plan 模式不是新的执行引擎，而是 prompt injection 驱动的行为模式切换**——同一个 LLM、同一个 loop、同一套工具，通过注入指令改变模型行为。

关键代码路径：
- `SessionWorker`（`worker.go:32`）— 会话级状态
- `processSession`（`processor.go:830`）— 组装 messages、registry、systemPrompt，调用 `RunAgentLoop`
- `BuildSystemPrompt`（`processor.go:62`）— 构建 system prompt
- `registerWecomBuiltinTools`（`wecom_builtin_tools.go:28`）— 注册 wecom 工具
- `UpdateSessionContextAndCursor`（`sessions.go:225`）— 持久化 session 状态

## Goals / Non-Goals

**Goals:**

- LLM 能自主判断何时进入计划模式（通过调用 `enter_plan_mode` 工具）
- 计划模式下 LLM 只收集信息、不执行操作，产出计划文本发给用户
- 用户在微信中回复确认/修改意见后，agent 按计划执行或修订
- 模式状态跨 `processSession` 调用持久化（用户可能隔一段时间才回复）
- 实现尽量轻量，不改 `RunAgentLoop` 和 `ToolRegistry` 的核心接口

**Non-Goals:**

- 不做 registry 层面的工具移除/硬约束（prompt 约束已足够）
- 不做显式的状态机（`awaiting_approval`、`executing` 等中间状态由对话上下文自然处理）
- 不做多阶段 agent 嵌套（Claude Code 的 5-phase explore/design agent 工作流过重）
- 不改 `AgentLoopConfig` 或 `RunAgentLoop` 签名

## Decisions

### D1: 模式状态存在 SessionWorker 上，持久化到 session context

**选择**：在 `SessionWorker` 上增加 `Mode SessionMode` 字段（`"normal"` / `"plan"`），在 `processSession` 结束时随 session context 一起持久化。

**替代方案**：
- 在 `SessionData` 增加 DB 字段：需要 migration，当前只有 `normal`/`plan` 两个值，不值得改 schema
- 在 `SessionData.Context` JSON 里嵌入 mode：与对话历史耦合，解析复杂

**理由**：`SessionWorker` 是内存中的会话状态载体，`Mode` 是会话级状态。持久化时序列化为 session metadata（如写入一个独立的 session setting 或 context 附属字段），恢复时从中读取。使用 `storage.UpdateSession` 的 `executionStatus` 字段（已有机制）来承载 mode 信息，或新增 `mode` 列，取决于改造成本——优先复用 `executionStatus`。

### D2: 行为控制完全通过 prompt injection，不做 registry 层拦截

**选择**：当 `mode == "plan"` 时，在 `BuildSystemPrompt` 返回值末尾追加一段 plan 模式约束指令。工具注册不变，所有工具仍然可见。

**替代方案**：
- 在 plan 模式下从 registry 移除写工具：增加了 `ToolRegistry` 的复杂度，需要支持"条件注册"
- 在 executor 层拦截（plan 模式下写工具返回错误）：安全但增加耦合

**理由**：Claude Code 的生产验证表明 prompt 约束对现代 LLM 已经足够可靠。保持 registry 简单性。如果未来发现 LLM 在 plan 模式下仍调写工具，再加 executor 层拦截作为兜底。

### D3: enter_plan_mode / exit_plan_mode 作为 builtin tool 注册

**选择**：在 `processSession` 的工具注册阶段，注册两个 builtin tool：
- `enter_plan_mode`：LLM 调用时设置 `worker.Mode = "plan"`，返回确认信息
- `exit_plan_mode`：LLM 调用时将计划文本通过 `send_channel_message` 发给用户，然后设置 `worker.Mode = "normal"`

**替代方案**：
- 不做显式工具，只在 system prompt 里写"复杂任务先写计划"：LLM 不知道何时切换上下文约束，计划和执行阶段的指令混在一起
- 做 3 个工具（enter/submit/exit 分开）：submit 和 exit 的拆分没有实际意义，增加认知负担

**理由**：两个工具 = 两个状态转换点，语义清晰。`enter_plan_mode` 是进入（模型判断）、`exit_plan_mode` 是退出（计划完成，发给用户）。退出后自然回到 normal，用户的回复在下一轮 `processSession` 中作为普通消息处理——LLM 看到之前的计划 + 用户确认，自然知道该执行了。

### D4: exit_plan_mode 通过已有的 send_channel_message 路径发送计划

**选择**：`exit_plan_mode` 的 executor 接收 `plan` 参数（计划文本），内部复用 `send_channel_message` 的发送逻辑将计划发给用户，然后切换 mode。

**替代方案**：
- LLM 自己先调 `wecom_send_message` 再调 `exit_plan_mode`：plan 模式下 wecom_send_message 被 prompt 约束禁止使用，逻辑矛盾
- 不发送计划，只在对话中返回：用户在微信端看不到完整计划

**理由**：退出计划模式的核心动作就是"把计划发给用户等确认"，合并为一个工具调用最自然。

### D5: plan 模式约束指令的内容

约束指令注入到 system prompt 末尾，核心内容：

```
你当前处于计划模式。在这个模式下：
- 只使用只读工具收集信息（wecom_search_targets, wecom_list_or_get_conversations, 
  wecom_get_group_detail, wecom_get_contact_detail, inspect_attachment）
- 不要调用任何会产生副作用的工具（wecom_send_message, wecom_revoke_message, 
  wecom_manage_group, send_channel_message, execute_command 等）
- 充分理解任务后，制定清晰的执行计划
- 计划就绪后调用 exit_plan_mode 工具将计划发送给用户
```

指令只在 `mode == "plan"` 时追加，normal 模式下不注入任何额外内容。

### D6: enter_plan_mode 的触发时机由工具 description 引导

**选择**：在 `enter_plan_mode` 的 tool description 中写明触发条件（影响多目标、不可逆操作、需求模糊等），由 LLM 自主判断是否调用。

**替代方案**：
- 在 system prompt 中写触发规则：始终占用 prompt 空间，即使大部分任务不需要 plan
- 代码层面做复杂度评估：过度工程化，LLM 的判断力已经足够

**理由**：工具 description 只在 LLM 看到工具列表时生效，不额外消耗 prompt tokens。且遵循 Claude Code 的做法——让模型自主判断。

## Risks / Trade-offs

- **[风险] LLM 在 plan 模式下仍调用写工具** → 缓解：prompt 约束在现代 LLM 上可靠性高；如果发生，后续迭代加 executor 层拦截（D2 已预留）
- **[风险] LLM 对 enter_plan_mode 的触发判断不准（太多/太少）** → 缓解：通过调整 tool description 的触发条件文案迭代优化；初期偏保守（只在高风险场景触发）
- **[风险] 用户回复后 LLM 丢失计划上下文** → 缓解：计划文本在对话历史中（exit_plan_mode 的 tool result），`processSession` 恢复历史时自然带入
- **[取舍] 不做硬约束意味着理论上 LLM 可以绕过** → 接受：简单性优先，与 Claude Code 的设计选择一致
- **[取舍] 没有中间状态（awaiting_approval）** → 接受：退出 plan 模式后就是 normal，用户的确认/修改在下一轮作为普通消息处理，LLM 从上下文理解意图——不需要额外状态机
