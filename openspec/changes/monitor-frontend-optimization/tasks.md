## 1. 后端：Compaction API

- [x] 1.1 在 `agent/internal/api/router.go` 注册 `GET /api/agent-sessions/{id}/compactions` 路由
- [x] 1.2 实现 handler：调用 `storage.ListCompactions(sessionID)`，返回 JSON 数组
- [x] 1.3 编译验证：`cd agent && go build ./...`

## 2. 后端：Trace 事件增强

- [x] 2.1 在 `processOneEvent` 的 absorb 分支中增加 `logger.Business(ctx, "消息吸纳", "traceEvent", "absorb", "absorbRound", ..., "absorbedCount", ...)`
- [x] 2.2 在 `processOneEvent` 的 compact 成功分支中增加 `logger.Business(ctx, "上下文压缩", "traceEvent", "compact", "tokensBefore", ..., "tokensAfter", ..., "archivedCount", ...)`
- [x] 2.3 在 `traces.go` 的 `buildTrace` 中增加对 `traceEvent: "absorb"` 和 `traceEvent: "compact"` 的处理，生成对应的 ExecutionStep
- [x] 2.4 编译验证：`cd agent && go build ./...`

## 3. 前端：类型和 API 层

- [x] 3.1 在 `admin/src/api/types.ts` 中新增 `CompactionData` 类型，为 `ExecutionStep` 的 type 增加 `"absorb" | "compact"` 联合类型
- [x] 3.2 在 `admin/src/api/sessions.ts` 中新增 `getCompactions(sessionId)` 方法
- [x] 3.3 删除 `admin/src/api/logs.ts`，从 `types.ts` 移除 `LogEntry`
- [x] 3.4 从 `admin/src/hooks/use-monitor.ts` 移除 `useRecentTraces`

## 4. 前端：Hooks 层

- [x] 4.1 创建 `monitor/hooks/use-llm-io.ts`：统一 `useLLMIO(traceId, ref)` hook，query key `["llm-io", traceId, ref]`，`staleTime: Infinity`
- [x] 4.2 创建 `monitor/hooks/use-session-compactions.ts`：`useSessionCompactions(sessionId)` hook

## 5. 前端：三栏布局骨架

- [x] 5.1 创建 `monitor/components/` 和 `monitor/hooks/` 和 `monitor/lib/` 目录结构
- [x] 5.2 重写 `monitor-page.tsx` 为三栏布局壳：SessionList + ConversationTimeline + DecisionInspector，实现右栏折叠/展开
- [x] 5.3 提取 `session-list.tsx`：从原 MonitorPage 中提取搜索、过滤、session 列表渲染

## 6. 前端：Conversation Timeline

- [x] 6.1 创建 `monitor/lib/build-timeline.ts`：实现 `buildTimeline(messages, compactions, traces)` 函数，按时间合并消息、压缩事件、吸纳事件
- [x] 6.2 创建 `monitor/lib/types.ts`：定义 `TimelineEvent` 联合类型和 monitor 局部类型
- [x] 6.3 创建 `conversation-timeline.tsx`：渲染 TimelineEvent 列表，用户消息和 Agent 回复为对话气泡
- [x] 6.4 创建 `timeline-event.tsx`：单个时间线事件组件，处理消息/压缩/吸纳/processing 四种类型
- [x] 6.5 创建 `compaction-event.tsx`：压缩事件的时间线标记（归档数、token 变化）
- [x] 6.6 实现 exchange 点击选中逻辑：点击 timeline 中的 exchange 更新 Inspector 选中状态

## 7. 前端：Decision Inspector

- [x] 7.1 创建 `decision-inspector.tsx`：接收选中 exchange 的 trace，渲染 stats + round tabs + round detail
- [x] 7.2 创建 `trace-stats-bar.tsx`：总耗时、tokens、成本、LLM 调用数、迭代数
- [x] 7.3 创建 `round-detail.tsx`：单轮执行详情——thinking + model output + tools + errors + compaction
- [x] 7.4 提取 `thinking-view.tsx`：从原 ThinkingView 提取，默认展开
- [x] 7.5 创建 `model-output-view.tsx`：使用 `useLLMIO` hook 提取并展示模型文本输出
- [x] 7.6 创建 `tool-card.tsx`：工具调用卡片（input + result 并排），从原 ToolPairView 重构
- [x] 7.7 提取 `llm-io-viewer.tsx`：LLM I/O 弹窗，从原 LLMIOViewer 提取
- [x] 7.8 实现 absorb round tab 逻辑：从 trace steps 中识别 absorb 事件，按 round 分组 steps，单轮时不显示 tabs

## 8. 前端：加载态和收尾

- [x] 8.1 创建 `exchange-skeleton.tsx`：对话加载骨架屏
- [x] 8.2 实现 auto-scroll：新消息到达时自动滚动到底部，手动上滚时暂停
- [x] 8.3 实现 auto-select：session processing 时自动选中最新 exchange
- [x] 8.4 验证所有原有功能正常：session 搜索、删除、trace 展开、LLM I/O 查看、markdown 渲染
