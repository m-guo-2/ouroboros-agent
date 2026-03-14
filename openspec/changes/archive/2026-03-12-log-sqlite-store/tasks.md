## 1. 接口定义与类型

- [x] 1.1 在 `shared/logger/` 新增 `store.go`，定义 `LogWriter`、`LogReader` 接口及 `TraceFilter`、`TraceSummary`、`TraceEvent` 类型
- [x] 1.2 定义 `Level` 常量保持不变，确保接口与现有 Level 兼容

## 2. FileStore 实现

- [x] 2.1 在 `shared/logger/` 新增 `store_file.go`，将现有 `writeToFile` 和 JSONL 文件操作封装为 `FileStore` struct 实现 `LogWriter`
- [x] 2.2 `FileStore.WriteLLMIO` 封装现有 `WriteLLMIO` 的文件写入逻辑
- [x] 2.3 `FileStore.Close` 关闭所有打开的文件句柄

## 3. SQLiteStore 实现

- [x] 3.1 添加 `modernc.org/sqlite` 依赖
- [x] 3.2 在 `shared/logger/` 新增 `store_sqlite.go`，实现 SQLiteStore struct 含 writer db、readers map、dir 路径
- [x] 3.3 实现建库逻辑：`ensureDB(date)` 创建 `{logDir}/sqlite/{date}.db`，建 events 表（含 trace_id/session_id/agent_id/user_id/channel/level/event/severity/iteration/model/tool/msg/timestamp/data 列）、llm_io 表、全部索引（含 partial index），开启 WAL 模式
- [x] 3.4 实现 `LogWriter.Append`：从 entry map 提取 traceId/sessionId/agentId/userId/channel/traceEvent/severity/iteration/model/tool/msg 到对应索引列，data 存完整 JSON
- [x] 3.5 实现 `LogWriter.WriteLLMIO`：写入当日 db 的 llm_io 表
- [x] 3.6 实现 `LogReader.ListTraces`：按日倒序查询，支持 session_id 过滤和 limit，跨日聚合
- [x] 3.7 实现 `LogReader.ReadTraceEvents`：按 trace_id 索引查询，跨日合并按 timestamp 排序
- [x] 3.8 实现 `LogReader.ReadLLMIO` 和 `ListLLMIORefs`：按 ref/trace_id 索引查询
- [x] 3.9 实现日切换：检测当前日期变化时创建新 db，旧 writer 降级为 reader
- [x] 3.10 实现连接池 LRU 淘汰：reader 数量超过 7 时关闭最久未使用的连接
- [x] 3.11 实现 `Cleanup`：扫描 sqlite/ 目录，关闭过期 db 连接后删除文件
- [x] 3.12 实现 `Close`：关闭 writer 和所有 reader 连接

## 4. 异步写入管道

- [x] 4.1 在 `shared/logger/` 新增 `async.go`，实现 `asyncWriter` struct 含 buffered channel (cap 4096) 和后台消费 goroutine
- [x] 4.2 `asyncWriter.Append` 非阻塞发送到 channel，buffer 满时丢弃并 stderr 告警
- [x] 4.3 `asyncWriter.WriteLLMIO` 同样走异步 channel
- [x] 4.4 实现 `multiWriter`：fan-out 到多个 `LogWriter`，单个失败不影响其他
- [x] 4.5 `asyncWriter.Close/Flush` 排空 channel 后关闭下游 multiWriter

## 5. 重构 logger.go

- [x] 5.1 修改 `Init()` 签名或新增 `InitWithStore()`：创建 FileStore + SQLiteStore，组合为 multiWriter，包装为 asyncWriter
- [x] 5.2 修改 `writeLog` 调用链：通过 asyncWriter 写入，保留 `writeToConsole` 同步调用
- [x] 5.3 修改 `WriteLLMIO` 通过 asyncWriter 写入
- [x] 5.4 修改 `Flush()` 调用 asyncWriter.Flush 排空并关闭
- [x] 5.5 导出 `GetReader() LogReader` 供 traces API 使用
- [x] 5.6 保留现有 `cleanupLoop` 逻辑，增加调用 SQLiteStore.Cleanup

## 6. 重构 traces.go

- [x] 6.1 修改 `tracesHandler` 持有 `LogReader` 而非 `logDir string`
- [x] 6.2 重写 `listTraces`：调用 `reader.ListTraces(filter)` 替代 readBusinessJSONL 全量扫描
- [x] 6.3 重写 `buildTrace`：调用 `reader.ReadTraceEvents(traceID)` 获取已过滤事件，只做结构转换
- [x] 6.4 重写 `serveLLMIO` 和 `listLLMIORefs`：调用 reader 对应方法
- [x] 6.5 保留 `completedTraceCache` 逻辑不变（缓存已构建的 executionTrace）
- [x] 6.6 删除 `readBusinessJSONL`、`getAvailableDates` 等文件直读函数

## 7. 接入点与配置

- [x] 7.1 修改 `agent/internal/api/router.go` 的 `Mount` 签名：接收 `LogReader` 替代 `logDir string`
- [x] 7.2 修改 `agent/cmd/agent/main.go`：初始化 logger 后通过 `logger.GetReader()` 获取 reader 传给 Mount
- [x] 7.3 在 `deploy/config/agent.yaml` 增加说明注释（无需新配置项，双写默认开启）

## 8. 前端 — 中间面板统一外部事件标签

- [x] 8.1 修改 `conversation-timeline.tsx`：去掉 `isSystemInitiated` 的紫色"系统"样式，统一使用"外部事件"标签和一致的图标/颜色
- [x] 8.2 修改 `build-timeline.ts` 的 `buildExchanges`：系统触发消息不再显示"(系统触发)"占位文本，改为显示实际消息内容

## 9. 前端 — Decision Inspector 扁平事件流

- [x] 9.1 在 `lib/build-timeline.ts` 新增 `flattenSteps(steps)` 函数：将 steps 按 timestamp 排序，合并同一 iteration 的 thinking + llm_call 为一个"模型输出"事件，tool_call 和 tool_result 分别为独立事件
- [x] 9.2 更新 `lib/types.ts`：新增 `FlatEvent` 类型（type: 模型输出/工具执行/工具结果/错误，含关联 step 引用）
- [x] 9.3 重写 `round-detail.tsx`：去掉 Iteration 分组折叠，改为遍历 `FlatEvent[]` 渲染扁平列表，每行默认收起，点击展开详情
- [x] 9.4 "模型输出"行：收起时显示 thinking 摘要（截断），展开显示完整 thinking + llm_call 指标（model、token、耗时、费用）+ LLM I/O 入口
- [x] 9.5 "工具执行"行：收起时显示工具名称和耗时，展开显示输入参数 JSON
- [x] 9.6 "工具结果"行：收起时显示成功/失败状态和结果摘要，展开显示完整返回 JSON
- [x] 9.7 "错误"行：红色标签，展开显示完整错误信息
- [x] 9.8 `decision-inspector.tsx`：保留 Round 标签页（absorb 场景）和 TraceStatsBar，Round 内部传入扁平事件流
- [x] 9.9 保留 `LLMIOViewer` 组件不变，作为模型输出展开后的详情入口

## 10. 链路补全

- [x] 10.1 在 `traces.go` 的 `buildTrace` switch 中补充 `empty_response_retry` 和 `attachment_guard` 事件处理，使其出现在 trace 步骤中
- [x] 10.2 `start` 事件中 dispatcher 额外写了一条带 traceEvent=start 的 "消息派发" 日志，确认 buildTrace 能正确处理同一 traceId 的多条 start 事件（取第一条）

## 11. 验证

- [x] 11.1 启动服务，确认日志同时写入 JSONL 文件和 SQLite db 文件
- [ ] 11.2 通过 Monitor 页面验证 trace 列表和详情正常加载
- [ ] 11.3 验证 LLM I/O 查看功能正常
- [ ] 11.4 验证前端：外部事件标签统一显示、扁平事件流正确渲染、点击展开详情正常
- [ ] 11.5 验证日切换：模拟跨日场景，确认新建 db 和查询跨日 trace 正常
- [ ] 11.6 验证清理：确认过期 db 文件被正确删除
- [ ] 11.7 用 `sqlite3` 命令行验证索引列数据正确（trace_id/session_id/user_id/channel/model/tool 等字段已正确提取）
