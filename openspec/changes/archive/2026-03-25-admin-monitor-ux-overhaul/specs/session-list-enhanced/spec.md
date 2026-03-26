## ADDED Requirements

### Requirement: Session 状态筛选
Session 列表 SHALL 支持按 `executionStatus` 筛选会话。

#### Scenario: 筛选 processing 状态
- **WHEN** 用户选择 status filter 为 "processing"
- **THEN** 列表只显示 `executionStatus === "processing"` 的会话
- **THEN** 后端 API 使用 `?status=processing` 参数过滤

#### Scenario: 筛选 error 状态
- **WHEN** 用户选择 status filter 为 "error"
- **THEN** 列表只显示 `executionStatus === "error"` 的会话

#### Scenario: 默认不筛选
- **WHEN** 页面初始加载
- **THEN** 显示所有状态的会话（无 status filter）

### Requirement: Session 服务端搜索
Session 列表 SHALL 支持服务端搜索，搜索范围包含会话标题和频道名称。

#### Scenario: 输入搜索关键词
- **WHEN** 用户在搜索框输入 "test"
- **THEN** 前端以 debounce（300ms）方式调用 `GET /api/agent-sessions?search=test`
- **THEN** 后端返回 title 或 channelName 中包含 "test" 的会话

#### Scenario: 清空搜索框
- **WHEN** 用户清空搜索框内容
- **THEN** 恢复无搜索条件的完整列表

### Requirement: 后端 API 扩展 status 和 search 参数
`GET /api/agent-sessions` SHALL 新增 `status` 和 `search` 查询参数。

#### Scenario: status 参数过滤
- **WHEN** 请求 `GET /api/agent-sessions?status=processing`
- **THEN** 返回的 sessions 列表中所有项的 `executionStatus` 均为 `"processing"`

#### Scenario: search 参数模糊搜索
- **WHEN** 请求 `GET /api/agent-sessions?search=hello`
- **THEN** 返回 title 或 channel_name 中包含 "hello"（大小写不敏感）的 sessions

#### Scenario: 多参数组合
- **WHEN** 请求 `GET /api/agent-sessions?agentId=a1&status=processing&search=test`
- **THEN** 返回同时满足所有条件的 sessions

### Requirement: 错误 Session 视觉标记
Session 列表中 `executionStatus` 为 "error" 的会话 SHALL 有醒目的视觉标记。

#### Scenario: session 状态为 error
- **WHEN** 列表中存在 `executionStatus === "error"` 的会话
- **THEN** 该会话行左侧显示红色指示条或红色图标，使其在列表中一眼可辨

### Requirement: 自定义删除确认 Dialog
删除 session 操作 SHALL 使用自定义 AlertDialog 替代浏览器原生 `confirm()`。

#### Scenario: 点击删除按钮
- **WHEN** 用户点击某 session 的删除按钮
- **THEN** 弹出自定义 AlertDialog，包含：红色警告图标、会话名称、"此操作不可恢复" 提示、"取消" 和 "删除" 两个按钮（删除按钮为红色）

#### Scenario: 确认删除
- **WHEN** 用户在 AlertDialog 中点击 "删除"
- **THEN** 执行删除操作，关闭 Dialog

#### Scenario: 取消删除
- **WHEN** 用户点击 "取消" 或按 Escape 或点击蒙层
- **THEN** 关闭 Dialog，不执行任何操作

### Requirement: 绝对时间 Tooltip
Session 列表和对话时间线中所有 `timeAgo` 显示 SHALL 在 hover 时展示绝对时间。

#### Scenario: hover 相对时间
- **WHEN** 用户将鼠标悬停在 "3 分钟前" 文字上
- **THEN** Tooltip 显示完整的绝对时间（如 "2026-03-25 14:32:05"）
