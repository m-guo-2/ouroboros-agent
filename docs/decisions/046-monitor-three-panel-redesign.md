# Monitor 可观测性前端三栏布局重设计

- **日期**：2026-03-05
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

Monitor 页面是系统可观测性的唯一入口。引入 absorb-replan 循环（ADR 045）后，Agent 的处理周期从"用户消息 → 一次 LLM 调用 → 回复"变为多轮 Execute → Checkpoint → Absorb 循环。原有的两栏单文件设计（962 行 `monitor-page.tsx`）存在：对话流和决策过程混在一起相互干扰、上下文压缩完全不可见、吸纳轮次不可见、LLM 思考藏在两层折叠后。

## 决策

将信息架构从"单列 exchange 列表"重构为**三栏布局**，分离"沟通视角"和"决策视角"：

- **左栏 (Session List)**：会话导航
- **中栏 (Conversation Timeline)**：纯净对话流 + 压缩/吸纳事件作为一等时间线事件
- **右栏 (Decision Inspector)**：选中 exchange 的完整决策过程（stats、thinking、tools、absorb round tabs）

## 变更内容

### 后端（Go）
- `agent/internal/api/sessions.go`：新增 `GET /api/agent-sessions/{id}/compactions` 端点
- `agent/internal/runner/processor.go`：absorb 和 compact 分支增加 `traceEvent` 字段
- `agent/internal/api/traces.go`：`executionStep` 增加 absorb/compact 专用字段，`buildTrace` 增加对应 case

### 前端（TypeScript/React）
- `admin/src/components/features/monitor/monitor-page.tsx`：从 962 行单文件重写为三栏布局壳（~120 行）
- 新增 `monitor/components/`：session-list、conversation-timeline、decision-inspector、round-detail、thinking-view、tool-card、model-output-view、llm-io-viewer、trace-stats-bar、compaction-event、exchange-skeleton（11 个组件文件）
- 新增 `monitor/hooks/`：use-llm-io（统一缓存 LLM I/O）、use-session-compactions
- 新增 `monitor/lib/`：types（本地类型）、build-timeline（exchange 构建、round 分割、iteration 分组）
- `admin/src/api/types.ts`：ExecutionStep 增加 absorb/compact 类型，新增 CompactionData，移除 LogEntry
- `admin/src/api/sessions.ts`：新增 getCompactions
- 删除 `admin/src/api/logs.ts`（死代码），移除 `useRecentTraces`（未使用）

## 考虑过的替代方案

- **保持两栏，在 exchange 内嵌详情**：无法分离沟通和决策两个信息通道。
- **用 accordion 代替 tabs 展示 absorb rounds**：accordion 暗示同一流程的不同部分，与多轮独立执行的语义不符。

## 影响

- 压缩事件和吸纳轮次首次在 UI 中可见，运维人员可以看到 Agent 的完整处理周期
- LLM I/O 通过统一 hook + React Query 缓存去重，消除了重复网络请求
- 组件从 1 个 962 行文件拆分为 15 个文件，最大文件 ~170 行，可维护性大幅提升
