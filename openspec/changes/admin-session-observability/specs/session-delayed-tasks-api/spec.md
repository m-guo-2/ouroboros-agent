## ADDED Requirements

### Requirement: Session delayed tasks read API

The system SHALL expose `GET /api/agent-sessions/{sessionId}/delayed-tasks` returning delayed tasks for the specified session.

The response SHALL use the standard envelope `{ "success": true, "data": [...] }` where each item contains `id` (number), `sessionId` (string), `agentId` (string), `userId` (string), `channel` (string), `channelUserId` (string), `channelConversationId` (string), `task` (string), `executeAt` (number, unix ms), `status` (string: pending/dispatched/cancelled), `createdAt` (number, unix ms), `updatedAt` (number, unix ms).

Tasks SHALL be ordered by `execute_at DESC` (most recent first).

#### Scenario: List all tasks for a session

- **WHEN** a GET request is sent to `/api/agent-sessions/{sessionId}/delayed-tasks` without a `status` query parameter
- **THEN** the API returns HTTP 200 with all delayed tasks for that session regardless of status

#### Scenario: Filter tasks by status

- **WHEN** a GET request is sent to `/api/agent-sessions/{sessionId}/delayed-tasks?status=pending`
- **THEN** the API returns HTTP 200 with only pending tasks for that session

#### Scenario: Session has no tasks

- **WHEN** a GET request is sent to `/api/agent-sessions/{sessionId}/delayed-tasks` and the session has no delayed tasks
- **THEN** the API returns HTTP 200 with an empty array `[]`

### Requirement: DelayedTask JSON serialization

The `DelayedTask` struct SHALL have `json` tags using camelCase naming to match frontend conventions.

#### Scenario: JSON field names are camelCase

- **WHEN** a delayed task is serialized to JSON
- **THEN** field names use camelCase (e.g. `sessionId`, `executeAt`, `channelUserId`)

### Requirement: ListDelayedTasksBySession supports status filter

The storage layer SHALL provide a `ListDelayedTasksBySession(sessionID, status string)` function that:
- Returns all tasks when `status` is empty or `"all"`
- Returns only tasks with the specified status otherwise
- Orders results by `execute_at DESC`

#### Scenario: Query all statuses

- **WHEN** `ListDelayedTasksBySession` is called with `status=""` or `status="all"`
- **THEN** all tasks for the session are returned regardless of status

#### Scenario: Query specific status

- **WHEN** `ListDelayedTasksBySession` is called with `status="dispatched"`
- **THEN** only tasks with `status='dispatched'` for the session are returned
