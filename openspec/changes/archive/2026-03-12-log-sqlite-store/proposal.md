## Why

当前日志查看基于 JSONL 文件全量读取，每次 API 请求都将整个日文件加载到内存并逐行解析。随着数据增长，business 日文件可达数十 MB，导致 Monitor 页面打不开、接口超时。同时日志写入直接在业务协程中执行文件 I/O，存在阻塞主流程的风险。

## What Changes

**后端 — 日志存储层重构：**
- 在 `shared/logger` 层引入 `LogStore` 存储后端接口，解耦日志读写与底层存储实现
- 实现 `SQLiteStore` 后端：按日拆分独立 `.db` 文件，建立 trace_id/session_id 索引，查询走索引而非全文件扫描
- 保留现有 `FileStore`（JSONL）后端，两个后端同时双写
- 日志写入改为异步（channel buffer），不阻塞业务主流程
- 重构 `agent/internal/api/traces.go`，从 `LogStore` 接口读取，移除直接文件操作
- 日志清理：SQLite 按日库直接删文件，无需 DELETE + VACUUM

**前端 — Monitor 事件流扁平化：**
- 去掉"系统触发"标签，统一使用"外部事件"
- Decision Inspector 右侧面板从三层嵌套（Round → Iteration → Step）改为按时间顺序的扁平事件流
- 事件类型统一为：外部事件、模型输出、工具执行、工具结果、错误
- 所有事件行默认收起，点击展开查看详情（模型 token/耗时/I/O、工具输入/输出）

## Capabilities

### New Capabilities
- `log-store-interface`: LogStore 抽象接口定义，支持多后端写入和统一读取
- `sqlite-log-backend`: SQLite 按日拆分存储后端实现，含异步写入和索引查询
- `monitor-flat-timeline`: Monitor 前端事件流扁平化，去掉系统触发和 Iteration 分组

### Modified Capabilities

## Impact

**后端：**
- `shared/logger/logger.go` — 写入路径重构为通过 LogStore 接口异步写入
- `agent/internal/api/traces.go` — 读取路径重构为通过 LogStore 接口查询
- `agent/internal/api/router.go` — Mount 签名变化，传入 LogStore 而非 logDir 字符串
- `agent/internal/config/config.go` — 新增日志后端配置项
- `agent/cmd/agent/main.go` — 初始化 LogStore 并注入
- 新增依赖：`modernc.org/sqlite`（纯 Go SQLite，无 CGO）
- 部署配置 `deploy/config/agent.yaml` — 新增 `log.backend` 配置

**前端：**
- `admin/src/components/features/monitor/components/conversation-timeline.tsx` — 去掉"系统触发"，统一为"外部事件"
- `admin/src/components/features/monitor/components/round-detail.tsx` — 重写为扁平事件流
- `admin/src/components/features/monitor/lib/build-timeline.ts` — 简化 `groupStepsByIteration` 为 `flattenSteps`
- `admin/src/components/features/monitor/components/decision-inspector.tsx` — 适配扁平化结构
- `admin/src/components/features/monitor/lib/types.ts` — 更新类型定义
