## ADDED Requirements

### Requirement: Monitor page tab navigation

The Monitor page center column SHALL display a tab bar below the session header with three tabs: "对话" (Conversation), "记忆" (Memory), "定时任务" (Tasks). The default active tab SHALL be "对话".

#### Scenario: Tab switching

- **WHEN** the user clicks on the "记忆" tab
- **THEN** the center column replaces the conversation timeline with the session memory panel

#### Scenario: Default tab

- **WHEN** a session is selected in the Monitor page
- **THEN** the "对话" tab is active and the conversation timeline is displayed

### Requirement: Session memory panel

The session memory panel SHALL display all memory facts for the selected session, fetched from `GET /api/agent-sessions/{id}/facts`.

Each fact SHALL show: the fact text, category badge, and relative creation time.

Facts SHALL be grouped by category if multiple categories exist.

#### Scenario: Display facts

- **WHEN** the "记忆" tab is active and the session has facts
- **THEN** all facts are displayed with fact text, category, and creation time

#### Scenario: Empty state

- **WHEN** the "记忆" tab is active and the session has no facts
- **THEN** a placeholder message "此会话暂无记忆" is shown

#### Scenario: Manual refresh

- **WHEN** the user clicks the refresh button on the memory panel
- **THEN** the facts data is re-fetched from the API

### Requirement: Delayed tasks panel

The delayed tasks panel SHALL display all delayed tasks for the selected session, fetched from `GET /api/agent-sessions/{id}/delayed-tasks`.

Each task SHALL show: task content, planned execution time (formatted), and status badge (color-coded: pending=yellow, dispatched=green, cancelled=gray).

The panel SHALL provide a status filter (All / Pending / Dispatched / Cancelled).

#### Scenario: Display tasks

- **WHEN** the "定时任务" tab is active and the session has tasks
- **THEN** all tasks are displayed with content, execution time, and status badge

#### Scenario: Filter by status

- **WHEN** the user selects "Pending" in the status filter
- **THEN** only pending tasks are displayed

#### Scenario: Empty state

- **WHEN** the "定时任务" tab is active and the session has no tasks
- **THEN** a placeholder message "此会话暂无定时任务" is shown

### Requirement: Data fetching with React Query

The memory and tasks panels SHALL use React Query hooks with `enabled` controlled by the active tab, so data is only fetched when the corresponding tab is visible.

`staleTime` SHALL be set to 30 seconds.

#### Scenario: Lazy loading

- **WHEN** the "对话" tab is active
- **THEN** no requests are made to the facts or delayed-tasks APIs

#### Scenario: Fetch on tab switch

- **WHEN** the user switches to the "记忆" tab for the first time
- **THEN** a request is made to `GET /api/agent-sessions/{id}/facts`
