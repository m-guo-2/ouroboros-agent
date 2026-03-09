## ADDED Requirements

### Requirement: Human checkpoint tool
系统 SHALL 提供 `request_human_intervention` 工具，使 browser subagent 能在人工关卡前暂停执行并请求用户协助。

输入:
- `description` (string, required): 需要用户执行的动作说明
- `screenshot` (bool, optional, default true): 是否附带当前页面截图
- `timeout_minutes` (int, optional, default 5): 等待超时时间

输出: JSON 对象，至少包含:
- `resolved` (bool)
- `user_message` (string)
- `elapsed_ms` (int)

#### Scenario: 请求用户扫码登录
- **WHEN** subagent 调用 `request_human_intervention` 且描述为“请扫描页面二维码完成登录”
- **THEN** 系统向用户发送说明消息
- **AND** subagent 进入等待态

#### Scenario: 用户完成后恢复
- **WHEN** checkpoint 正在等待中，用户回复任意确认消息
- **THEN** 工具返回 `resolved=true`
- **AND** 返回中包含用户回复内容
- **AND** subagent 从暂停点继续执行

#### Scenario: 超时未恢复
- **WHEN** checkpoint 等待超过 `timeout_minutes`
- **THEN** 工具返回 `resolved=false`
- **AND** pending checkpoint 被清理

### Requirement: Checkpoint notification delivery
系统 SHALL 通过现有 `channels.SendToChannel` 机制发送人工 checkpoint 通知，不新增专用消息通道。

通知消息 SHALL 包含:
1. 需要用户做什么
2. 为什么需要用户介入
3. 完成后如何恢复（例如“回复任意消息继续”）
4. 页面截图（如果 `screenshot=true`）

#### Scenario: 发送到原始渠道
- **WHEN** 当前 session 来源于 feishu、qiwei 或 webui
- **THEN** checkpoint 通知发送到同一个来源渠道与会话

### Requirement: Pending checkpoint routing
系统 SHALL 维护 session 级别的 pending checkpoint 状态。当 session 存在 pending checkpoint 时，用户的下一条回复 SHALL 优先用于解除 checkpoint，而不是进入新的 agent 对话。

#### Scenario: 用户回复优先解除 checkpoint
- **WHEN** session 存在 pending checkpoint 且用户发送新消息
- **THEN** 系统调用 checkpoint resolve 流程
- **AND** 该消息 SHALL NOT 作为新的 agent input 进入主 loop

#### Scenario: 无 pending checkpoint 时正常对话
- **WHEN** session 不存在 pending checkpoint 且用户发送新消息
- **THEN** 消息按现有流程进入 agent 处理

### Requirement: Checkpoint manager
系统 SHALL 提供一个进程内 manager 管理人工 checkpoint 的创建、等待、恢复与超时清理。

manager SHALL 至少支持:
- `Request(...)`
- `Resolve(...)`
- `PendingForSession(...)`

#### Scenario: 通过 API 解除 checkpoint
- **WHEN** 调用 `POST /api/interventions/{id}/resolve`
- **THEN** 对应的 waiting checkpoint 被解除
- **AND** subagent 恢复执行

#### Scenario: 请求被取消
- **WHEN** subagent 在等待 checkpoint 时被取消
- **THEN** `Request(...)` 立即返回取消错误
- **AND** pending checkpoint 被清理

### Requirement: Browser subagent concurrency control
系统 SHALL 对 browser subagent 设置独立的并发限制，默认同时运行数量上限为 2。

#### Scenario: 达到并发上限
- **WHEN** 已有 2 个 browser subagent 正在运行，又发起新的 browser subagent
- **THEN** 系统返回并发上限错误

#### Scenario: 名额释放后继续运行
- **WHEN** 某个 browser subagent 完成并释放资源
- **THEN** 新的 browser subagent 可以正常启动

### Requirement: Remote takeover is not required for V1
系统 V1 SHALL NOT 依赖 VNC、noVNC 或其他远程桌面能力作为 checkpoint 的必要条件。

#### Scenario: 仅依赖原始渠道恢复
- **WHEN** V1 browser subagent 遇到人工关卡
- **THEN** 系统仅依赖消息通知、截图和用户回复完成恢复流程
