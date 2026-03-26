## 1. 基础设施：URL 状态持久化

- [x] 1.1 创建 `useMonitorSearchParams` hook，封装 useSearchParams，提供 session/tab/exchange 的读写方法
- [x] 1.2 在 MonitorPage 中接入 hook，替换 useState 为 URL params 驱动：selectedSessionId、activeTab、selectedExchangeIndex
- [x] 1.3 处理边界情况：URL 中 session 不存在时回退默认值并清除参数；切换 session 时清除 exchange 参数

## 2. 基础设施：实时轮询策略

- [x] 2.1 修改 `useMonitorSessions` hook，根据是否存在 processing session 动态设置 refetchInterval（5s / 30s），页面非前台时停止轮询
- [x] 2.2 修改 `useSessionMessages` hook，当 session 为 processing 状态时设置 3s refetchInterval
- [x] 2.3 修改 trace query，当 trace 状态为 running 时设置 3s refetchInterval，完成后停止
- [x] 2.4 在 TraceStatsBar 中增加 useEffect + setInterval，running 状态时每秒更新时长显示

## 3. 基础设施：timeAgo 自动刷新

- [x] 3.1 创建 `useTimeAgoTick` hook，每 60 秒触发一次 tick state 变化
- [x] 3.2 在 SessionList、ConversationTimeline 中接入 tick，驱动 timeAgo 重新计算
- [x] 3.3 为所有 timeAgo 显示增加 title 属性，hover 时展示绝对时间（YYYY-MM-DD HH:mm:ss）

## 4. Session 列表增强

- [x] 4.1 后端：`listSessions` 函数扩展 `status` 查询参数，在 SQL WHERE 中增加 `execution_status = ?` 条件
- [x] 4.2 后端：`listSessions` 函数扩展 `search` 查询参数，在 SQL WHERE 中增加 `(title LIKE ? OR channel_name LIKE ?)` 条件（参数化查询）
- [x] 4.3 前端：SessionList 组件增加 status filter chips（全部 / processing / idle / error），选择后触发带 status 参数的 API 请求
- [x] 4.4 前端：将搜索逻辑从客户端过滤改为服务端搜索，增加 300ms debounce，调用 API 时携带 search 参数
- [x] 4.5 前端：error 状态的 session 卡片增加红色左边框或红色图标标记
- [x] 4.6 搜索状态下保留 "加载更多" 按钮（移除 `!search` 条件限制）

## 5. 删除确认 Dialog

- [x] 5.1 创建 DeleteSessionDialog 组件（基于 Radix AlertDialog），包含红色警告样式、会话名称显示、不可恢复提示
- [x] 5.2 在 MonitorPage 中替换 `confirm()` 调用为 DeleteSessionDialog

## 6. 对话时间线 UX 改进

- [x] 6.1 替换统一的 "外部事件" 标签为语义化标签：根据 initiator 字段区分 "用户消息" / "系统触发" / "定时任务" / 其他
- [x] 6.2 每条消息（用户消息和助手消息）增加 hover 时显示的复制按钮，点击后复制消息纯文本内容
- [x] 6.3 实现 "滚动到底部" 浮动按钮：距底部 > 200px 时显示，点击后 smooth scroll 到底部
- [x] 6.4 修复 CompactionEvent 的交互一致性：移除 cursor-pointer 样式（因为 onClick 未接入），或者为 compaction 事件增加展开 summary 的交互

## 7. Decision Inspector 升级

- [x] 7.1 将标题 "Decision Inspector" 改为 "决策详情"
- [x] 7.2 在 Inspector header 中增加 "全部展开" / "全部折叠" 按钮，通过 context 或 state 控制所有 step 的 expanded 状态
- [x] 7.3 修改 ErrorRow 和 ToolResultRow（toolSuccess === false），使其默认 expanded 为 true
- [x] 7.4 将 ToolCard / RoundDetail 中的英文标签统一为中文（Input→输入, Result→结果, Error→错误, Tool→工具）

## 8. Tab Count Badge

- [x] 8.1 在 MonitorPage 中预取各 tab 的数据计数（facts count、delayed tasks count、subagent jobs count）
- [x] 8.2 修改 TABS 渲染逻辑，在 tab label 旁显示数量 badge（非零时显示）

## 9. Subagent 执行详情补全

- [x] 9.1 前端：创建 `useSubagentJobDetail` hook，调用 `GET /api/subagent-jobs/{id}` 获取完整 job 详情
- [x] 9.2 修改 SubagentJobsPanel：job 卡片改为可展开，展开时显示完整 task、result（Markdown 渲染）、error（红色区域）、impacts 列表
- [x] 9.3 修改 SubagentJobsPanel：task 描述从 CSS truncate 改为默认 2 行 line-clamp，支持展开全文
- [x] 9.4 MonitorPage 中为 SubagentJobsPanel 接入 trace 跳转：传入 onViewTrace callback，点击后切换到 conversation tab、选中对应 exchange、Inspector 中打开 subagent trace
- [x] 9.5 修改 DecisionInspector 支持 2 层 subagent trace 钻取：将 subagentView 从单个对象改为栈结构，面包屑支持多层级导航返回

## 10. 代码清理

- [x] 10.1 将 `safePretty` 和 `escapeHtml` 从 round-detail.tsx 和 tool-card.tsx 提取到共享 utils
- [x] 10.2 移除 tool-card.tsx 中的 `openJsonInNewTab` 重复定义，统一引用
