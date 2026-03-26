## ADDED Requirements

### Requirement: Session facts read API

The system SHALL expose `GET /api/agent-sessions/{sessionId}/facts` returning all memory facts for the specified session, ordered by creation time ascending.

The response SHALL use the standard envelope `{ "success": true, "data": [...] }` where each item contains `id` (number), `sessionId` (string), `fact` (string), `category` (string), `createdAt` (number, unix ms).

#### Scenario: Session has facts

- **WHEN** a GET request is sent to `/api/agent-sessions/{sessionId}/facts` and the session has saved facts
- **THEN** the API returns HTTP 200 with all facts ordered by `createdAt ASC`

#### Scenario: Session has no facts

- **WHEN** a GET request is sent to `/api/agent-sessions/{sessionId}/facts` and the session has no saved facts
- **THEN** the API returns HTTP 200 with an empty array `[]`

#### Scenario: Invalid session ID

- **WHEN** a GET request is sent to `/api/agent-sessions//facts` (empty ID)
- **THEN** the API returns HTTP 400 with `{ "success": false, "error": "missing session id" }`
