## ADDED Requirements

### Requirement: Compaction HTTP API

The agent SHALL expose an HTTP endpoint to retrieve compaction history for a session.

#### Scenario: List compactions for session

- **WHEN** a GET request is made to `/api/agent-sessions/{id}/compactions`
- **THEN** the response SHALL contain an array of compaction records with fields: id, summary, archivedMessageCount, tokenCountBefore, tokenCountAfter, compactModel, createdAt

#### Scenario: No compactions

- **WHEN** a session has no compaction history
- **THEN** the response SHALL return an empty array

### Requirement: Compaction trace event

The agent runner SHALL emit a business log entry with `traceEvent: "compact"` when context compaction occurs during processing.

#### Scenario: Compact event in JSONL

- **WHEN** CompactContext succeeds and compacted is true
- **THEN** a business log entry SHALL be written with `traceEvent: "compact"`, `tokensBefore`, `tokensAfter`, `archivedCount`, and `summary` fields

#### Scenario: Trace builder processes compact events

- **WHEN** the trace builder encounters a `traceEvent: "compact"` log entry
- **THEN** it SHALL create an ExecutionStep with `type: "compact"` containing the compaction metadata

### Requirement: Frontend compaction data

The admin frontend SHALL fetch and display compaction data for the selected session.

#### Scenario: Fetch compactions

- **WHEN** a session is selected in the monitor
- **THEN** the frontend SHALL fetch compaction data via `GET /api/agent-sessions/{id}/compactions`
- **AND** merge compaction events into the conversation timeline by timestamp
