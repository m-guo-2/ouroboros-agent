## Context

Admin Monitor 当前展示三列布局：Session 列表 → 对话时间线 → 决策检查器。数据源覆盖 messages、compactions、traces，但遗漏了两类运行时状态：

1. **Session Memory**（`session_facts` 表）：agent 通过 `save_memory` 工具或压缩流程写入的记忆条目，运行时注入 LLM 上下文的 `[Session Memory]` 块。当前无 HTTP API 暴露给 admin。
2. **Delayed Tasks**（`delayed_tasks` 表）：agent 通过 `set_delayed_task` 工具创建的定时任务，由 30s 轮询调度器执行。当前无 HTTP API 暴露给 admin。

后端存储层已有完整的读写函数（`GetSessionFacts`、`ListPendingTasksBySession`），只需补充 API handler 和前端 UI。

## Goals / Non-Goals

**Goals:**

- 在 admin 中查看任意 session 的所有记忆条目（fact + category + 时间）
- 在 admin 中查看任意 session 的定时任务列表（任务内容、计划执行时间、当前状态）
- 定时任务支持按状态过滤（pending / dispatched / cancelled / all）
- UI 融入现有 Monitor 页面，不破坏已有布局

**Non-Goals:**

- 不提供记忆的增删改操作（admin 为只读观测）
- 不提供定时任务的手动触发或取消操作
- 不做跨 session 的全局定时任务视图（本次只做 session 维度）
- 不做记忆或任务的搜索/过滤（记忆数量通常有限）

## Decisions

### D1: API 路由挂载在 session 子路径下

将新 API 作为 session 的子资源：
- `GET /api/agent-sessions/{id}/facts`
- `GET /api/agent-sessions/{id}/delayed-tasks[?status=pending|dispatched|cancelled]`

**理由**：与已有的 `/messages`、`/compactions` 子路径保持一致，语义清晰。在 `handleSessionsWithID` 中添加 sub-path 分支即可，无需新增顶层路由。

**替代方案**：独立路由 `/api/session-facts?sessionId=xxx`。放弃——与现有 pattern 不一致，增加前端 API 调用的心智负担。

### D2: 定时任务查询函数扩展为支持全状态

现有 `ListPendingTasksBySession` 只查 `pending`。新增 `ListDelayedTasksBySession(sessionID, status string)` 函数：
- `status` 为空或 `"all"` 时返回所有状态的任务
- 否则按指定状态过滤
- 按 `execute_at DESC` 排序（最近的排前面）

**理由**：运维需要看到已执行和已取消的任务来排查问题，只看 pending 不够。

### D3: `DelayedTask` 结构体补充 JSON tags

现有 `DelayedTask` 没有 JSON tags，直接序列化会产生 PascalCase 字段名。补充 `json:"camelCase"` tags 对齐前端命名约定。

### D4: UI 采用 session header 下方 tab 切换

在 Monitor 中间列的 session header 下方增加轻量 tab 栏（对话 / 记忆 / 定时任务），默认显示"对话"（即现有的 ConversationTimeline）。

**理由**：
- tab 切换比抽屉/弹窗更直观，不遮挡其他面板
- 记忆和任务信息量不大，不需要独立页面
- 与三列布局自然融合

**替代方案**：放在右侧 DecisionInspector 面板内。放弃——Inspector 已经信息密集，加入会显得拥挤。

### D5: 数据获取策略

- 使用 React Query，在选中 session 且切到对应 tab 时才触发请求（`enabled` 控制）
- `staleTime` 设为 30s（记忆和任务不会高频变化）
- 不做轮询，提供手动刷新按钮

## Risks / Trade-offs

- **[性能]** session_facts 没有数量上限 → 长期运行的 session 可能积累大量 facts → **缓解**：API 默认 limit=200，前端分页加载（本次先不做分页，因为实际量级通常 <100 条，后续按需加）
- **[一致性]** 定时任务状态可能在查看时变化（被调度器 dispatch） → **缓解**：只读展示 + 手动刷新按钮，不做实时推送
