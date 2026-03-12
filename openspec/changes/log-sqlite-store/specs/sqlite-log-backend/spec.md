## ADDED Requirements

### Requirement: Per-day SQLite database files

The SQLiteStore SHALL create one SQLite database file per calendar day under `{logDir}/sqlite/{date}.db`.

Each database SHALL contain:
- `events` table with indexed columns: `trace_id`, `session_id`, `agent_id`, `user_id`, `channel`, `level`, `event`, `severity`, `iteration`, `model`, `tool`, `msg`, `timestamp`; and a `data` column storing the full JSON entry
- `llm_io` table with columns: `ref` (primary key), `trace_id`, `iteration`, `data` (blob)
- Indexes: `events(trace_id)`, `events(session_id)`, `events(level, event)`, partial indexes on `events(user_id)`, `events(channel)`, `events(severity)` where not empty, `llm_io(trace_id)`

The `Append` method SHALL extract these fields from the entry map to populate indexed columns:
- `trace_id` ← entry["traceId"]
- `session_id` ← entry["sessionId"]
- `agent_id` ← entry["agentId"]
- `user_id` ← entry["userId"]
- `channel` ← entry["channel"]
- `event` ← entry["traceEvent"]
- `severity` ← entry["severity"]
- `iteration` ← entry["iteration"]
- `model` ← entry["model"]
- `tool` ← entry["tool"]
- `msg` ← entry["msg"]
- `level` and `timestamp` from function parameters

#### Scenario: First write of the day
- **WHEN** the first log entry of a new calendar day arrives
- **THEN** SQLiteStore SHALL create `{date}.db`, create tables and indexes, and insert the entry

#### Scenario: Day rollover at midnight
- **WHEN** a log entry arrives after midnight
- **THEN** SQLiteStore SHALL create a new day's database and switch the active writer to it

### Requirement: Indexed trace queries

SQLiteStore SHALL implement `LogReader` with indexed queries instead of full-table scans.

#### Scenario: ListTraces performance
- **WHEN** ListTraces is called with limit=50
- **THEN** the query SHALL use the `trace_id` index and return within 100ms even when the day's database contains 100K+ events

#### Scenario: ReadTraceEvents by trace ID
- **WHEN** ReadTraceEvents is called with a traceID
- **THEN** the query SHALL use `idx_events_trace` index to retrieve only matching rows, ordered by timestamp

#### Scenario: Cross-day trace query
- **WHEN** a trace's events span two calendar days (e.g., started before midnight, completed after)
- **THEN** ReadTraceEvents SHALL query both days' databases and merge results ordered by timestamp

### Requirement: Multi-day reader with on-demand connections

SQLiteStore SHALL maintain a pool of read-only database connections, opened on demand.

#### Scenario: Query recent traces across days
- **WHEN** ListTraces needs more results than today's database contains
- **THEN** it SHALL open yesterday's database (read-only), query it, and continue until limit is satisfied or no more databases exist

#### Scenario: Connection lifecycle
- **WHEN** a day's database is opened for reading
- **THEN** it SHALL be cached for reuse and opened in read-only mode (`?mode=ro`)

#### Scenario: Connection limit
- **WHEN** more than 7 reader connections are open
- **THEN** the least recently used connection SHALL be closed to limit resource usage

### Requirement: Database cleanup by file deletion

SQLiteStore SHALL clean up old data by deleting entire database files.

#### Scenario: Retention enforcement
- **WHEN** the cleanup routine runs
- **THEN** SQLite database files with dates older than the configured retention period SHALL be deleted after closing any open connections

#### Scenario: No VACUUM needed
- **WHEN** old data needs to be purged
- **THEN** the system SHALL delete the `.db` file directly instead of running DELETE + VACUUM queries

### Requirement: SQLite WAL mode for concurrent access

SQLiteStore SHALL enable WAL (Write-Ahead Logging) mode on all database connections.

#### Scenario: Concurrent read during write
- **WHEN** the background writer goroutine is inserting events
- **THEN** API read queries SHALL proceed without blocking, via WAL mode

### Requirement: LLM I/O stored in SQLite

SQLiteStore SHALL store LLM I/O data in the `llm_io` table of the corresponding day's database, alongside events.

#### Scenario: Write LLM I/O
- **WHEN** WriteLLMIO is called with ref, traceID, iteration, and data
- **THEN** the data SHALL be inserted into the `llm_io` table of the current day's database

#### Scenario: Read LLM I/O
- **WHEN** ReadLLMIO is called with a ref
- **THEN** it SHALL search recent days' databases for the ref and return the data blob

#### Scenario: List LLM I/O refs
- **WHEN** ListLLMIORefs is called with a traceID
- **THEN** it SHALL query the `llm_io` table's `idx_llmio_trace` index across recent days and return sorted refs
