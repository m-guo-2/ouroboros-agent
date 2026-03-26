## ADDED Requirements

### Requirement: Session 列表智能轮询
Monitor 页面 SHALL 根据是否存在 processing 状态的 session 动态调整 session 列表的轮询间隔。

#### Scenario: 存在 processing session
- **WHEN** session 列表中存在至少一个 `executionStatus === "processing"` 的会话
- **THEN** session 列表每 5 秒自动刷新一次

#### Scenario: 没有 processing session
- **WHEN** session 列表中所有会话的 `executionStatus` 都不是 `"processing"`
- **THEN** session 列表每 30 秒自动刷新一次

#### Scenario: 页面不在前台时停止轮询
- **WHEN** 浏览器标签页不在前台（document.hidden === true）
- **THEN** 暂停所有轮询，回到前台后恢复

### Requirement: 选中 Session 消息列表智能轮询
当前选中的 session 处于 processing 状态时，消息列表 SHALL 自动高频轮询更新。

#### Scenario: 选中的 session 正在 processing
- **WHEN** 用户选中了一个 `executionStatus === "processing"` 的 session
- **THEN** 消息列表每 3 秒自动刷新
- **THEN** 新消息出现时自动滚动到底部（如果用户之前在底部）

#### Scenario: 选中的 session 已完成
- **WHEN** 用户选中的 session 的 `executionStatus` 不是 `"processing"`
- **THEN** 消息列表不自动轮询

### Requirement: Trace 数据智能轮询
当选中的 trace 处于 running 状态时，trace 数据 SHALL 自动高频轮询更新。

#### Scenario: 选中的 trace 为 running
- **WHEN** Decision Inspector 中展示的 trace 状态为 `"running"`
- **THEN** trace 数据每 3 秒自动刷新

#### Scenario: trace 变为 completed 或 error
- **WHEN** 轮询返回的 trace 状态变为 `"completed"` 或 `"error"`
- **THEN** 停止轮询

### Requirement: 运行时长实时更新
TraceStatsBar SHALL 对 running 状态的 trace 实时更新持续时长显示。

#### Scenario: trace 正在运行
- **WHEN** trace 状态为 `"running"`
- **THEN** 时长显示每秒更新一次

#### Scenario: trace 已完成
- **WHEN** trace 状态不是 `"running"`
- **THEN** 时长显示为固定值（completedAt - startedAt），不再更新
