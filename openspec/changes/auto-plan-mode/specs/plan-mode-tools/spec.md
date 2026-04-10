## ADDED Requirements

### Requirement: enter_plan_mode builtin tool
系统 SHALL 注册一个名为 `enter_plan_mode` 的 builtin tool，LLM 可自主调用以进入计划模式。

该工具的 description SHALL 明确列出触发条件：
- 操作影响多个目标（多个群、多个联系人）
- 不可逆操作（批量发消息、群管理）
- 用户请求模糊，需要先确认范围和方案
- 涉及敏感操作（清理成员、批量通知）

该工具的 description 也 SHALL 列出不需要 plan 的场景：
- 简单查询
- 单条消息回复
- 用户指令非常明确且影响范围小

该工具 SHALL 不接受任何输入参数。

#### Scenario: LLM 面对复杂任务时调用 enter_plan_mode
- **WHEN** LLM 判断当前任务满足触发条件并调用 enter_plan_mode
- **THEN** SessionWorker.Mode SHALL 被设置为 `"plan"`，tool result SHALL 返回确认文本指引 LLM 进入计划流程

#### Scenario: 已在 plan 模式下重复调用
- **WHEN** SessionWorker.Mode 已经是 `"plan"` 且 LLM 再次调用 enter_plan_mode
- **THEN** tool result SHALL 返回提示"已在计划模式中"，Mode 保持 `"plan"`

### Requirement: exit_plan_mode builtin tool
系统 SHALL 注册一个名为 `exit_plan_mode` 的 builtin tool，LLM 在计划完成后调用以将计划发送给用户。

该工具 SHALL 接受一个必填参数 `plan`（string），为计划文本。

调用时 SHALL 执行两个动作：
1. 将 plan 文本通过当前会话的消息发送通道发给用户
2. 将 SessionWorker.Mode 设置为 `"normal"`

#### Scenario: 正常退出计划模式
- **WHEN** LLM 在 plan 模式下调用 exit_plan_mode 并提供 plan 文本
- **THEN** 计划文本 SHALL 被发送到当前微信会话，SessionWorker.Mode SHALL 变为 `"normal"`，tool result SHALL 确认计划已发送

#### Scenario: 非 plan 模式下调用
- **WHEN** SessionWorker.Mode 为 `"normal"` 且 LLM 调用 exit_plan_mode
- **THEN** tool result SHALL 返回错误提示"当前不在计划模式中"

#### Scenario: plan 参数为空
- **WHEN** LLM 调用 exit_plan_mode 但 plan 参数为空字符串
- **THEN** tool result SHALL 返回错误提示"计划内容不能为空"
