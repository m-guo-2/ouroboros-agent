## Why

管理后台 Monitor 观测页面存在大量体验问题，严重影响运维和调试效率。核心痛点：URL 状态不持久化导致刷新丢失调试上下文；没有实时推送导致观测 processing 会话需要反复手动刷新；Subagent 执行过程展示不完整（tab 与 trace 未打通、结果/错误不可见）；Session 列表缺乏筛选能力难以定位目标会话；Decision Inspector 的 JSON 查看体验粗糙、默认全折叠降低调试效率。这些问题导致 Monitor 页面从"可用"到"好用"之间有明显差距。

## What Changes

- **URL 状态持久化**：将选中的 session ID、active tab、selected exchange index 同步到 URL search params，支持刷新保持、链接分享
- **实时数据更新**：对 processing 状态的 session 实现智能轮询（消息列表、trace 数据），活跃会话高频刷新，非活跃低频
- **Session 列表增强**：增加按 agent / channel / status 筛选，服务端搜索支持，错误 session 视觉标记，删除操作用自定义 Dialog 替代 `confirm()`
- **对话时间线改进**：用户消息与系统触发用不同标签区分（替代统一的"外部事件"），增加消息复制、滚动到底部按钮，timeAgo 自动刷新
- **Decision Inspector 升级**：增加全部展开/折叠按钮，错误 step 自动展开，运行中 trace 时长实时更新，中英文标签统一
- **Subagent 执行详情补全**：SubagentJobsPanel 接入 trace 跳转，展示 subagent 的 result / error / impacts，支持嵌套 subagent trace 钻取
- **Tab count badge**：记忆、定时任务、Subagent tab 显示数量

## Capabilities

### New Capabilities
- `monitor-url-state`: Session/tab/exchange 选择状态同步到 URL search params，支持刷新持久化和链接分享
- `monitor-realtime-polling`: 基于 session executionStatus 的智能轮询策略，processing 会话高频刷新消息和 trace
- `session-list-enhanced`: Session 列表筛选（agent/channel/status）、服务端搜索、错误标记、自定义删除确认 Dialog
- `conversation-timeline-ux`: 消息角色标签语义化、消息复制、滚动到底部、timeAgo 自动刷新
- `inspector-ux-upgrade`: 全部展开/折叠、错误自动展开、运行时长实时更新、标签中英统一、Tab count badge

### Modified Capabilities
- `subagent-execution-ui`: 补全 SubagentJobsPanel 的 trace 跳转（接入 onViewTrace）；增加 subagent result/error/impacts 展示；支持多层嵌套 trace 钻取；task 描述可展开查看全文

## Impact

- **前端**：`admin/src/components/features/monitor/` 下所有组件均有改动，新增 URL state hook、轮询 hook、删除确认 Dialog 组件
- **后端 API**：Session 列表接口可能需要扩展 status filter 参数；subagent job 详情接口需要返回 result/error/impacts
- **依赖**：无新增外部依赖（URL state 用 react-router 的 useSearchParams，轮询用 TanStack Query 的 refetchInterval）
