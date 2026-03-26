## ADDED Requirements

### Requirement: URL 持久化选中的 Session
Monitor 页面 SHALL 将当前选中的 session ID 同步到 URL search param `session`。页面加载时 SHALL 从 URL 恢复选中状态。

#### Scenario: 选中 session 后 URL 更新
- **WHEN** 用户点击 session 列表中的某个会话（ID 为 `abc123`）
- **THEN** URL 更新为 `/monitor?session=abc123`（保留其他已有 params）

#### Scenario: 刷新页面恢复选中 session
- **WHEN** 用户在 `/monitor?session=abc123` 刷新页面
- **THEN** 页面加载后自动选中 ID 为 `abc123` 的 session

#### Scenario: URL 中的 session 不存在
- **WHEN** URL 中 `session` 参数指向一个不存在的 session ID
- **THEN** 回退到默认行为（选中 processing 会话或第一个会话），并清除 URL 中的无效 session 参数

### Requirement: URL 持久化 Active Tab
Monitor 页面 SHALL 将当前 active tab 同步到 URL search param `tab`。

#### Scenario: 切换 tab 后 URL 更新
- **WHEN** 用户切换到 "记忆" tab
- **THEN** URL 更新为 `/monitor?session=xxx&tab=memory`

#### Scenario: 刷新页面恢复 tab
- **WHEN** 用户在 `/monitor?session=xxx&tab=tasks` 刷新页面
- **THEN** 页面加载后自动切换到 "定时任务" tab

#### Scenario: URL 中没有 tab 参数
- **WHEN** URL 为 `/monitor?session=xxx`（无 tab 参数）
- **THEN** 默认显示 "对话" tab

### Requirement: URL 持久化选中的 Exchange
Monitor 页面 SHALL 将选中的 exchange index 同步到 URL search param `exchange`。

#### Scenario: 选中 exchange 后 URL 更新
- **WHEN** 用户点击对话时间线中的第 3 个 exchange
- **THEN** URL 更新为 `/monitor?session=xxx&tab=conversation&exchange=3`

#### Scenario: 切换 session 时清除 exchange
- **WHEN** 用户切换到另一个 session
- **THEN** URL 中的 `exchange` 参数被清除
