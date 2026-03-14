## Context

当前 `shared/logger` 直接在业务协程中执行文件 I/O（`writeToFile` 持有全局 mutex）。`agent/internal/api/traces.go` 读取时将整个 JSONL 日文件加载到内存再逐行过滤，无索引。随着日活增长，单日 business JSONL 达数十 MB，导致 Monitor 页面超时打不开。

现有代码结构：
- 写入：`shared/logger/logger.go` → `writeToFile()` 直接 append JSONL
- 读取：`agent/internal/api/traces.go` → `readBusinessJSONL()` 全量读文件 → 内存过滤
- 清理：`cleanupLoop()` 按文件日期删除

## Goals / Non-Goals

**Goals:**
- 日志写入不阻塞业务主流程（异步 channel）
- 同时输出到文件（保留现有 JSONL 可调试性）和 SQLite（支持索引查询）
- 通过 `LogStore` 接口抽象存储后端，读取端统一通过接口查询
- SQLite 按日拆分独立 .db 文件，清理 = 删文件
- Monitor 页面查询从全文件扫描降为索引查询

**Non-Goals:**
- 不做 Elasticsearch 后端实现（接口预留扩展性即可）
- 不改变日志内容格式和语义
- 不改变前端 Monitor 页面代码
- 不做历史 JSONL 数据迁移到 SQLite

## Decisions

### 1. 异步写入架构

写入路径：`logger.writeLog()` → channel → 后台 goroutine → 依次调用各 backend

```
Business goroutine
       │
       ▼ (non-blocking send)
   channel buffer (cap: 4096)
       │
       ▼ (background writer goroutine)
  ┌────┴────┐
  ▼         ▼
FileStore  SQLiteStore
(JSONL)    (per-day .db)
```

**选择 channel 而非 goroutine-per-write**：channel 有背压控制，buffer 满时可选择丢弃或阻塞，避免无限制创建 goroutine。buffer 设 4096 足以应对突发写入。

**LLM I/O 写入也走异步**：LLM I/O 数据较大（单次可达数百 KB），更不应阻塞业务流程。

### 2. 双写策略：文件 + SQLite 同时输出

不做"选择一个后端"的设计，而是 MultiWriter 同时写入所有注册的 backend：

```go
type multiWriter struct {
    backends []LogWriter
}

func (m *multiWriter) Append(level Level, entry map[string]any) error {
    var firstErr error
    for _, b := range m.backends {
        if err := b.Append(level, entry); err != nil && firstErr == nil {
            firstErr = err
        }
    }
    return firstErr
}
```

**不做事务性双写**：某个 backend 写失败不影响其他 backend，只记录错误。文件是主要的持久化保障，SQLite 是查询加速层。

### 3. 读取端只走 SQLite

读取接口 `LogReader` 只有一个实现（SQLiteStore），不从文件读取：
- 文件作为"归档/调试/备份"用途，人可以直接 `cat` / `jq` 查看
- API 查询全部走 SQLite 索引，保证性能

**为什么不做 FileStore 读取降级**：两套读取逻辑增加复杂度，且文件读取正是当前的性能瓶颈。如果 SQLite 不可用，Monitor 页面本身也没有意义。

### 4. SQLite 按日拆分

每天一个独立的 `{date}.db` 文件：

```
data/logs/
├── business/           # 现有 JSONL 文件（FileStore 继续写）
│   ├── 2026-03-10.jsonl
│   └── 2026-03-12.jsonl
├── detail/llm-io/      # 现有 LLM I/O JSON（FileStore 继续写）
├── sqlite/             # 新增 SQLite 目录
│   ├── 2026-03-10.db
│   ├── 2026-03-11.db
│   └── 2026-03-12.db   # 当前活跃写入库
```

每个 .db 表结构：

```sql
CREATE TABLE events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id   TEXT NOT NULL,
    session_id TEXT DEFAULT '',
    agent_id   TEXT DEFAULT '',
    user_id    TEXT DEFAULT '',
    channel    TEXT DEFAULT '',
    level      TEXT NOT NULL,
    event      TEXT DEFAULT '',
    severity   TEXT DEFAULT '',
    iteration  INTEGER DEFAULT 0,
    model      TEXT DEFAULT '',
    tool       TEXT DEFAULT '',
    msg        TEXT DEFAULT '',
    timestamp  TEXT NOT NULL,
    data       TEXT NOT NULL
);
CREATE INDEX idx_events_trace ON events(trace_id);
CREATE INDEX idx_events_session ON events(session_id);
CREATE INDEX idx_events_level_event ON events(level, event);
CREATE INDEX idx_events_user ON events(user_id) WHERE user_id != '';
CREATE INDEX idx_events_channel ON events(channel) WHERE channel != '';
CREATE INDEX idx_events_severity ON events(severity) WHERE severity != '';

CREATE TABLE llm_io (
    ref       TEXT PRIMARY KEY,
    trace_id  TEXT NOT NULL,
    iteration INTEGER DEFAULT 0,
    data      BLOB NOT NULL
);
CREATE INDEX idx_llmio_trace ON llm_io(trace_id);
```

**列提取策略**：把高频检索/过滤字段提取为独立列并建索引，大文本字段（thinking, toolInput, toolResult 等）留在 `data` JSON 中。具体：

| 提取为列 | 理由 |
|---|---|
| trace_id, session_id | 核心查询条件，几乎每个 API 都用 |
| agent_id, user_id, channel | 按用户/渠道/Agent 过滤 trace 列表 |
| level, event | 按日志级别和事件类型过滤 |
| severity | 快速定位 ERROR/WARN 条目 |
| iteration | trace 内步骤排序 |
| model, tool | 按模型/工具筛选（如"查看 tool X 的所有调用"） |
| msg | 日志消息文本，支持关键字搜索 |

| 留在 data JSON | 理由 |
|---|---|
| thinking, toolInput, toolResult | 大文本，只在详情页展开时读取 |
| inputTokens, outputTokens, costUsd, durationMs | 数值指标，展示用，极少做 WHERE 条件 |
| llmIORef, stopReason, toolCallId | 引用/元数据，查询频率低 |
| absorbRound, absorbedCount, tokensBefore, tokensAfter | 压缩/吸纳事件专属字段 |

**partial index**：`user_id`、`channel`、`severity` 使用 WHERE != '' 的部分索引，因为大部分事件这些字段为空，部分索引更省空间且查询更快。

### 5. 接口分离：LogWriter / LogReader

```go
type LogWriter interface {
    Append(level Level, entry map[string]any) error
    WriteLLMIO(ref string, traceID string, iteration int, data []byte) error
    Close() error
}

type LogReader interface {
    ListTraces(filter TraceFilter) ([]TraceSummary, error)
    ReadTraceEvents(traceID string) ([]TraceEvent, error)
    ReadLLMIO(ref string) ([]byte, error)
    ListLLMIORefs(traceID string) ([]string, error)
}
```

- `logger.go` 持有 `LogWriter`（实际是 asyncWriter → multiWriter）
- `traces.go` 持有 `LogReader`（实际是 SQLiteStore）
- `SQLiteStore` 同时实现 `LogWriter` + `LogReader`
- `FileStore` 只实现 `LogWriter`

### 6. 连接池与日切换

SQLiteStore 内部维护：
- `writer`: 当前日期的 db 连接（读写模式）
- `readers`: `map[string]*sql.DB` 按需打开只读连接，LRU 淘汰（最多保留 7 个）
- 午夜自动切换：新建当日 db，旧 writer 降级为 reader

### 7. 清理策略

- SQLiteStore：扫描 sqlite/ 目录，日期 < cutoff 的关闭连接后直接 `os.Remove`
- FileStore：保持现有逻辑不变
- 两者独立清理，互不影响

### 8. 前端 — 扁平事件流

当前 Decision Inspector 使用三层嵌套：Round（absorb 分隔）→ Iteration（LLM 调用轮次）→ Step（具体事件）。改为扁平化：

**数据变换**：`flattenSteps(steps: ExecutionStep[]) → FlatEvent[]`

将 steps 按 timestamp 排序后，映射为统一的 FlatEvent：

| Step type(s) | FlatEvent type | 收起摘要 | 展开详情 |
|---|---|---|---|
| `thinking` + 对应的 `llm_call` | 模型输出 | thinking 文本截断 80 字 | 完整 thinking + model/token/耗时/费用 + LLM I/O |
| `tool_call` | 工具执行 | 工具名 + 耗时 | 输入参数 JSON |
| `tool_result` | 工具结果 | 成功/失败 + 结果截断 | 完整返回 JSON |
| `error` | 错误 | 错误消息截断 | 完整错误文本 |
| `compact` | 保留在顶部统计区 | — | — |
| `absorb` | Round 分隔符 | — | — |

**关键合并逻辑**：同一 iteration 内的 `thinking` 步骤和 `llm_call` 步骤合并为一个"模型输出"事件。`llm_call` 不单独占一行，其指标（token、耗时）作为"模型输出"展开后的详情展示。

**中间面板**：去掉"系统"/"用户"区分，统一使用"外部事件"标签。保留消息内容和时间。

## Risks / Trade-offs

- **[新增 SQLite 依赖]** → `modernc.org/sqlite` 是纯 Go 实现，无 CGO 依赖，编译体积增加约 10MB，但免去交叉编译问题
- **[双写一致性]** → 不保证文件和 SQLite 完全一致（某个 backend 可能短暂失败）。可接受，因为文件是调试用途，SQLite 是查询用途，不需要强一致
- **[异步写入丢日志风险]** → 进程崩溃时 channel 中未消费的日志会丢失。mitigation：channel buffer 设合理大小（4096），Flush() 在优雅退出时排空 channel
- **[SQLite 并发写入]** → SQLite WAL 模式下单写多读没问题，异步 writer goroutine 保证串行写入
