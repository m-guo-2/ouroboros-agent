## ADDED Requirements

### Requirement: Clean conversation display

The conversation timeline SHALL display only communication-level events: user messages, agent responses, and system events. Trace details SHALL NOT appear in the timeline.

#### Scenario: User message display

- **WHEN** a session has user messages
- **THEN** each user message SHALL appear as a chat bubble with sender name, timestamp, and content

#### Scenario: Agent response display

- **WHEN** a session has assistant messages
- **THEN** each assistant message SHALL appear as a chat bubble with markdown rendering, timestamp, and a clickable indicator showing the exchange is inspectable

#### Scenario: Processing indicator

- **WHEN** a session is actively processing (executionStatus === "processing")
- **THEN** the timeline SHALL show an animated processing indicator at the bottom with a brief stats summary (iterations, tools used so far)

### Requirement: Compaction events in timeline

The conversation timeline SHALL display context compaction events as first-class timeline events, positioned chronologically between messages.

#### Scenario: Compaction event display

- **WHEN** a compaction occurred between two messages
- **THEN** a compaction marker SHALL appear in the timeline showing: archived message count, token change (before → after), and timestamp

#### Scenario: Compaction detail on click

- **WHEN** the user clicks a compaction event in the timeline
- **THEN** the decision inspector SHALL show the compaction details: full summary text, token counts, compact model used

### Requirement: Absorb events in timeline

The conversation timeline SHALL display message absorption events when the agent absorbed new messages during processing.

#### Scenario: Absorb event display

- **WHEN** a trace contains absorb steps (type === "absorb")
- **THEN** an absorb marker SHALL appear in the timeline showing: "N 条新消息被吸纳" with the absorb round number

### Requirement: Exchange selection

Clicking an exchange (user message or agent response) in the timeline SHALL select it for inspection in the Decision Inspector.

#### Scenario: Select exchange

- **WHEN** the user clicks on an agent response in the timeline
- **THEN** the corresponding exchange SHALL be highlighted in the timeline AND the Decision Inspector SHALL display that exchange's full trace detail

#### Scenario: Auto-select processing exchange

- **WHEN** a session is actively processing
- **THEN** the latest exchange SHALL be auto-selected and the Inspector SHALL show its live trace data
