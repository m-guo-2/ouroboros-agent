## Context

管理后台 Monitor 页面是 Agent 运维的核心观测工具。当前技术栈：React 19 + Vite 7 + TanStack Query + React Router 7 + Tailwind 4 + Radix UI。

现状问题（产品审计发现 30 项）：
- 所有页面状态（selected session/tab/exchange）仅存于 React state，刷新即丢失
- 没有实时数据推送，processing 会话需要手动刷新
- Subagent 执行详情展示不完整（tab 与 trace 未打通，result/error 不可见）
- Session 列表缺乏筛选和服务端搜索
- Decision Inspector 默认全折叠、JSON 无高亮、标签中英混用
- 删除操作用原生 `confirm()`、timeAgo 不自动刷新等大量细节问题

后端 API 现状：
- `GET /api/agent-sessions` 已支持 agentId/userId/channel 过滤，但不支持 status/search 过滤
- `GET /api/subagent-jobs/{id}` 已返回完整 Job 详情（含 result/error/impacts/events），前端未使用
- Session 列表截断 task 到 200 字符，前端无法获取完整 task

## Goals / Non-Goals

**Goals:**
- 刷新页面后保持调试上下文（URL state）
- Processing 会话自动更新，无需手动刷新
- Subagent 执行全链路可观测（列表 → 详情 → trace → 嵌套 trace）
- Session 列表可高效定位目标会话（多维筛选 + 服务端搜索）
- Decision Inspector 的调试效率显著提升（展开控制、错误高亮、实时时长）
- 所有文案统一为中文

**Non-Goals:**
- WebSocket/SSE 实时推送（本轮用智能轮询实现，后续可升级）
- JSON 语法高亮（用格式化 + monospace 保持可读性，不引入 highlight 库）
- 响应式 / 移动端适配（管理后台以桌面为主）
- Inspector 可拖拽调整宽度（本轮不做，用全屏模式替代）
- 深层嵌套 subagent trace（本轮支持 2 层，不做无限递归）

## Decisions

### D1: URL 状态管理方案 — useSearchParams

**选择**: 用 React Router 的 `useSearchParams` 将 session/tab/exchange 同步到 URL。

**备选**:
- 自定义 `window.history.replaceState`: 不与 React Router 集成，路由跳转时容易丢失
- 状态管理库（zustand persist to URL）: 引入不必要的复杂度

**理由**: 项目已使用 React Router 7，`useSearchParams` 是原生方案，零额外依赖。URL 格式：`/monitor?session=xxx&tab=conversation&exchange=2`。

### D2: 实时更新策略 — TanStack Query refetchInterval

**选择**: 利用 TanStack Query 的 `refetchInterval` 做条件轮询：
- Session 列表：非 processing 时 30s，有 processing 时 5s
- 选中 session 的消息列表：processing 时 3s，完成时停止
- 选中 trace：running 时 3s，完成时停止

**备选**:
- WebSocket/SSE: 需要后端新增推送通道，改动大，本轮不做
- 固定轮询间隔: 浪费资源

**理由**: TanStack Query 已有完善的轮询支持，条件轮询可以精准控制，只在需要时高频刷新。

### D3: Session 列表筛选 — 前端 filter bar + 后端扩展

**选择**: 
- 前端在搜索框下方增加 filter chips（agent / channel / status 三个维度）
- 后端 `GET /api/agent-sessions` 扩展 `status` 和 `search` 查询参数
- `status` filter 在 DB 层实现（WHERE execution_status = ?）
- `search` 在 DB 层用 LIKE 模糊匹配 title/channelName

**理由**: 后端已有 agentId/channel 过滤支持，只需增加 status 和 search。前端 client-side filter 在数据量大时不可靠。

### D4: Subagent 详情展示 — Job Detail API + 抽屉/内嵌面板

**选择**:
- SubagentJobsPanel 中每个 job 卡片点击后展开内嵌详情面板，调用 `GET /api/subagent-jobs/{id}` 获取完整数据
- 详情包含：完整 task、result、error、impacts 列表
- "查看执行过程" 按钮切换到 conversation tab 并自动选中对应的 exchange + 在 Inspector 中钻取到 subagent trace

**备选**:
- 直接在 Inspector 中展示所有 subagent 信息: Inspector 空间有限，不适合展示长文本 result
- 新开页面: 增加导航复杂度，不如内嵌

**理由**: 后端 API 已完整返回 Job 详情（result/error/impacts/events），前端只需要消费这些数据。内嵌展开保持单页上下文。

### D5: 删除确认 — 自定义 AlertDialog

**选择**: 使用 Radix UI 的 AlertDialog 替代 `confirm()`，红色警告样式，显示会话名称。

**理由**: 项目已依赖 Radix UI，AlertDialog 是现成组件。不可逆操作需要明确视觉警告。

### D6: timeAgo 自动刷新 — 自定义 useTimeAgo hook

**选择**: 封装 `useTimeAgo` hook，内部用 `setInterval` 每分钟触发一次 re-render。

**备选**:
- 依赖库 `timeago-react`: 引入新依赖解决一个小问题，不值得
- 不刷新: 时间显示不准确

**理由**: 实现简单（<20 行），无外部依赖。

## Risks / Trade-offs

- **[轮询频率与性能]** Processing 会话 3s 轮询消息列表可能对 SQLite 有压力 → 缓解：只在前台且 session 选中时轮询；TanStack Query 自动去重并发请求
- **[URL 状态参数过多]** exchange index 是数字，session ID 是长字符串，URL 会较长 → 缓解：session ID 可以只放前 8 位做匹配（列表中唯一即可），但为简单起见先放完整 ID
- **[后端 search 参数注入]** LIKE 查询需要防 SQL 注入 → 缓解：使用参数化查询（Go database/sql 的 `?` 占位符）
- **[Subagent Job 内存存储]** 当前 subagent manager 在内存中，重启后数据丢失 → 本轮不解决，属于已知限制

## Open Questions

- Session 列表是否需要排序选项（按时间/消息数/状态）？本轮默认按 updatedAt 倒序，后续可加。
