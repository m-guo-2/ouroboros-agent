# 增强可观测性：Skills 加载快照 + 迭代轮次追踪

- **日期**：2026-02-24
- **类型**：代码变更
- **状态**：已实施

## 背景

自研 ReAct Agent 引擎后，可观测性有了更多可以做的空间。之前的 Trace 系统虽然能记录 thinking/tool_call/tool_result，但缺少两个关键信息：

1. **Agent 启动时加载了哪些 Skills 和 Tools**——排查问题时不知道 Agent 执行时实际具备什么能力
2. **每一步属于第几轮迭代**——Server 端用启发式推断迭代号，与 Agent 实际循环轮次可能不一致

## 决策

- Agent 在完成配置加载和 ToolRegistry 构建后，上报一条 `source: "system"` 的 thinking 事件，内容包含模型、Skills 列表、全部可用工具列表
- Agent Loop 在每个事件（thinking/tool_call/tool_result/error）中附加 `iteration` 字段，由引擎直报
- Server 端 execution-trace 优先使用 Agent 上报的 iteration，回退到原有启发式推断
- Monitor 前端区分 system 步骤（紫色 Cpu 图标）和模型推理步骤（灰色 Brain 图标），并按迭代轮次分组显示

## 变更内容

- `agent/src/engine/types.ts`：AgentEvent 新增 `iteration?: number`
- `agent/src/engine/loop.ts`：所有 onEvent 调用附加当前 iteration
- `agent/src/engine/runner.ts`：配置加载后上报 Skills/Tools 快照
- `agent/src/services/server-client.ts`：TraceEventPayload 新增 `iteration`
- `server/src/services/execution-trace.ts`：TraceEvent 新增 `iteration`，handleTraceEvent 优先使用 Agent 直报值
- `admin/src/api/types.ts`：ExecutionStep 补充注释
- `admin/src/components/features/monitor/monitor-page.tsx`：system 步骤紫色区分、迭代分组标题、摘要统计显示轮次数

## 影响

- Monitor 页面查看执行详情时，可以直接看到 Agent 本次加载了哪些 Skills 和 Tools
- 每一步都有准确的迭代轮次号，方便定位"第几轮思考后调用了什么工具"
- system 类步骤（配置加载、循环启动）与模型推理步骤视觉区分，不会混淆
