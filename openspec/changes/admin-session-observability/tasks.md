## 1. 后端：存储层扩展

- [x] 1.1 为 `DelayedTask` 结构体添加 `json:"camelCase"` tags（`agent/internal/storage/delayed_tasks.go`）
- [x] 1.2 新增 `ListDelayedTasksBySession(sessionID, status string)` 函数，支持全状态查询和按状态过滤，按 `execute_at DESC` 排序

## 2. 后端：API Handler

- [x] 2.1 在 `agent/internal/api/sessions.go` 的 `handleSessionsWithID` 中添加 `facts` 和 `delayed-tasks` 子路径分支
- [x] 2.2 实现 `getSessionFacts` handler：调用 `storage.GetSessionFacts`，空结果返回 `[]`
- [x] 2.3 实现 `getSessionDelayedTasks` handler：读取 `?status=` 查询参数，调用 `storage.ListDelayedTasksBySession`，空结果返回 `[]`

## 3. 前端：API 层

- [x] 3.1 在 `admin/src/api/types.ts` 添加 `SessionFact` 和 `DelayedTask` 类型定义
- [x] 3.2 在 `admin/src/api/sessions.ts` 添加 `getFacts(id)` 和 `getDelayedTasks(id, status?)` 方法

## 4. 前端：React Query Hooks

- [x] 4.1 创建 `useSessionFacts(sessionId, enabled)` hook，staleTime 30s
- [x] 4.2 创建 `useSessionDelayedTasks(sessionId, status, enabled)` hook，staleTime 30s

## 5. 前端：Monitor UI 组件

- [x] 5.1 创建 `SessionMemoryPanel` 组件：展示 facts 列表（fact 文本、category badge、相对时间），空状态显示占位信息，支持手动刷新
- [x] 5.2 创建 `SessionDelayedTasksPanel` 组件：展示任务列表（任务内容、计划时间、状态 badge），提供状态过滤器，空状态显示占位信息
- [x] 5.3 在 `MonitorPage` session header 下方添加 tab 栏（对话 / 记忆 / 定时任务），根据 active tab 切换显示 ConversationTimeline / SessionMemoryPanel / SessionDelayedTasksPanel

## 6. 验证

- [x] 6.1 启动 agent 服务，通过 admin Monitor 页面验证三个 tab 切换正常、数据加载正确
- [x] 6.2 验证空 session（无记忆/无任务）显示占位状态
