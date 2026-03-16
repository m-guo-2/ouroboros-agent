## ADDED Requirements

### Requirement: API client SHALL throw on error responses
`fetchApi` MUST throw an Error when the HTTP response status is non-2xx or the response body contains `success: false`. The thrown Error MUST include the error message from the response.

#### Scenario: HTTP 500 response
- **WHEN** `fetchApi` receives an HTTP 500 response with body `{"success": false, "error": "database error"}`
- **THEN** `fetchApi` throws an Error with message containing "database error"

#### Scenario: HTTP 404 response
- **WHEN** `fetchApi` receives an HTTP 404 response
- **THEN** `fetchApi` throws an Error (not returns `{ success: false }`)

#### Scenario: React Query retry on API error
- **WHEN** a `queryFn` using `fetchApi` encounters an HTTP error
- **THEN** React Query detects the error and applies the configured `retry` policy

### Requirement: TypeScript types SHALL match backend JSON serialization
All timestamp fields (`createdAt`, `updatedAt`, `archivedBeforeTime`) in frontend TypeScript interfaces MUST be typed as `number`, matching the Go backend's `int64` JSON serialization.

#### Scenario: MessageData.createdAt is number
- **WHEN** the backend returns a message with `createdAt: 1710547200000`
- **THEN** the frontend TypeScript type accepts it as `number` without casting

#### Scenario: CompactionData.createdAt is number
- **WHEN** the backend returns a compaction with `createdAt: 1710547200000` and `archivedBeforeTime: 1710540000000`
- **THEN** both fields are typed as `number` in the frontend interface

#### Scenario: AgentSession.updatedAt is number
- **WHEN** the backend returns a session with `updatedAt: 1710547200000`
- **THEN** the field is typed as `number` in `AgentSession` and `AgentSessionListItem`

### Requirement: Messages page size SHALL be at least 50
The default page size for message pagination MUST be at least 50 to reduce round-trips and ensure a complete exchange is visible on first load.

#### Scenario: First page load
- **WHEN** the monitor page loads messages for a session
- **THEN** at least 50 messages are requested in the first API call

### Requirement: Trace query SHALL stop scanning after finding events
`ReadTraceEvents` MUST NOT scan all available date databases. After finding events for a trace ID, it SHALL check at most one additional day (for cross-midnight traces) and then stop.

#### Scenario: Trace spans single day
- **WHEN** `ReadTraceEvents` finds events in the 2026-03-15 database
- **THEN** it checks 2026-03-14 (one more day) and stops, without scanning 2026-03-13 and earlier

#### Scenario: No events found
- **WHEN** `ReadTraceEvents` finds no events in any database
- **THEN** it scans all available dates up to the configured limit (existing behavior)

### Requirement: Trace cache SHALL have a size limit
`completedTraceCache` MUST enforce a maximum entry count. When the limit is reached, the oldest entries SHALL be evicted.

#### Scenario: Cache reaches limit
- **WHEN** the cache contains 500 completed traces and a new trace is added
- **THEN** the oldest cached trace is evicted to make room

### Requirement: Database schema SHALL include channel_meta column
The `messages` table MUST have a `channel_meta TEXT` column. A migration MUST be added to create this column for existing databases.

#### Scenario: Fresh database
- **WHEN** the application starts with a fresh database
- **THEN** the `messages` table includes the `channel_meta` column

#### Scenario: Existing database without channel_meta
- **WHEN** the application starts with an existing database lacking `channel_meta`
- **THEN** the migration adds `channel_meta TEXT` to the `messages` table
