## ADDED Requirements

### Requirement: LogWriter interface for log storage backends

The system SHALL define a `LogWriter` interface in `shared/logger/` that all write-capable storage backends MUST implement.

The interface SHALL include:
- `Append(level Level, entry map[string]any) error` — 追加一条日志
- `WriteLLMIO(ref string, traceID string, iteration int, data []byte) error` — 写入 LLM I/O 数据
- `Close() error` — 关闭并释放资源

#### Scenario: FileStore implements LogWriter
- **WHEN** a FileStore is created with a log directory path
- **THEN** it SHALL implement LogWriter by appending JSONL to `{level}/{date}.jsonl` files, preserving current behavior

#### Scenario: SQLiteStore implements LogWriter
- **WHEN** a SQLiteStore is created with a log directory path
- **THEN** it SHALL implement LogWriter by inserting into the current day's SQLite database

### Requirement: LogReader interface for log query backends

The system SHALL define a `LogReader` interface in `shared/logger/` that query-capable storage backends MUST implement.

The interface SHALL include:
- `ListTraces(filter TraceFilter) ([]TraceSummary, error)` — 列出符合条件的 trace 摘要
- `ReadTraceEvents(traceID string) ([]TraceEvent, error)` — 读取指定 trace 的全部事件
- `ReadLLMIO(ref string) ([]byte, error)` — 读取 LLM I/O 数据
- `ListLLMIORefs(traceID string) ([]string, error)` — 列出指定 trace 的 LLM I/O 引用

`TraceFilter` SHALL support按 `SessionID`、`Limit`、`Days`（往前查几天）过滤。

#### Scenario: Query traces by session
- **WHEN** ListTraces is called with a SessionID filter
- **THEN** it SHALL return only traces belonging to that session, ordered by timestamp descending

#### Scenario: Query traces with limit
- **WHEN** ListTraces is called with Limit=20
- **THEN** it SHALL return at most 20 trace summaries, most recent first

#### Scenario: Read trace events
- **WHEN** ReadTraceEvents is called with a valid traceID
- **THEN** it SHALL return all events for that trace ordered by timestamp ascending

#### Scenario: Trace not found
- **WHEN** ReadTraceEvents is called with a nonexistent traceID
- **THEN** it SHALL return an empty slice and no error

### Requirement: Async write pipeline

The system SHALL write logs asynchronously via a buffered channel to avoid blocking the caller goroutine.

The pipeline SHALL:
- Accept log entries via a non-blocking channel send (buffer capacity: 4096)
- A single background goroutine SHALL consume from the channel and call each registered `LogWriter.Append` sequentially
- LLM I/O writes SHALL also go through the async pipeline

#### Scenario: Normal async write
- **WHEN** `logger.Business()` is called from a business goroutine
- **THEN** the entry SHALL be enqueued to the channel and the caller SHALL return immediately without waiting for I/O

#### Scenario: Channel buffer full
- **WHEN** the channel buffer is full (4096 entries pending)
- **THEN** the system SHALL drop the entry and log a warning to stderr, rather than blocking the caller

#### Scenario: Graceful shutdown
- **WHEN** `logger.Flush()` is called during process shutdown
- **THEN** it SHALL drain the channel, write all remaining entries to all backends, and close all backends

### Requirement: Multi-writer fan-out

The system SHALL support writing to multiple backends simultaneously.

#### Scenario: Dual write to file and SQLite
- **WHEN** both FileStore and SQLiteStore are registered as writers
- **THEN** every log entry SHALL be written to both backends

#### Scenario: One backend fails
- **WHEN** one backend returns an error during write
- **THEN** the other backend SHALL still receive the write, and the error SHALL be logged to stderr
