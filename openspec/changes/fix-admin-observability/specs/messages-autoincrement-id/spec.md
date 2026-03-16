## ADDED Requirements

### Requirement: messages table SHALL use auto-increment integer ID
The `messages` table MUST use `INTEGER PRIMARY KEY AUTOINCREMENT` as its ID column. The ID MUST be monotonically increasing and serve as the natural ordering for pagination.

#### Scenario: Insert message returns integer ID
- **WHEN** `SaveMessage` inserts a new message
- **THEN** it returns a `MessageData` with an integer `ID` assigned by the database

#### Scenario: ID ordering matches insertion order
- **WHEN** two messages are inserted sequentially
- **THEN** the second message's ID is greater than the first message's ID

### Requirement: Message pagination SHALL use integer ID as cursor
The `GET /api/agent-sessions/{id}/messages` endpoint MUST accept a `before` parameter as an integer message ID (not timestamp). Messages MUST be returned in ascending ID order (chronological).

#### Scenario: First page (no cursor)
- **WHEN** the endpoint is called without `before` parameter
- **THEN** it returns the latest N messages ordered by ID ascending

#### Scenario: Pagination with cursor
- **WHEN** the endpoint is called with `before=42`
- **THEN** it returns messages with `id < 42`, ordered by ID ascending, limited to page size

#### Scenario: No duplicate or skipped messages
- **WHEN** a client paginates through all messages using ID cursors
- **THEN** every message appears exactly once, with no gaps or duplicates

### Requirement: Frontend pagination SHALL use message ID as cursor
The frontend `useSessionMessages` hook MUST use the message `id` field (integer) as the pagination cursor instead of `createdAt`.

#### Scenario: getNextPageParam returns oldest message ID
- **WHEN** a page of messages is loaded
- **THEN** `getNextPageParam` returns the smallest `id` in the page as the cursor for the next page

### Requirement: Internal tables SHALL use auto-increment IDs
The following tables MUST use `INTEGER PRIMARY KEY AUTOINCREMENT`: `context_compactions`, `session_facts`, `user_memory_facts`, `user_channels`, `delayed_tasks`.

#### Scenario: Compaction insert returns integer ID
- **WHEN** `SaveCompaction` inserts a new compaction record
- **THEN** it returns a record with an integer ID

### Requirement: MessageData.ID type SHALL be integer
The Go `MessageData.ID` field MUST be `int64` (JSON: `number`). The frontend `MessageData.id` TypeScript type MUST be `number`.

#### Scenario: Backend JSON serialization
- **WHEN** a message is serialized to JSON
- **THEN** the `id` field is a JSON number (e.g., `42`), not a string

#### Scenario: Frontend type
- **WHEN** the frontend receives a message response
- **THEN** `MessageData.id` is typed as `number` in TypeScript

### Requirement: session_events.message_id SHALL reference integer ID
The `session_events` table's `message_id` column MUST store integer message IDs matching the new `messages.id` type.

#### Scenario: Event references message
- **WHEN** a session event is created referencing a message
- **THEN** the `message_id` column stores the integer ID of the referenced message
