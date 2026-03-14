## ADDED Requirements

### Requirement: Absorb trace event

The agent runner SHALL emit a business log entry with `traceEvent: "absorb"` when messages are absorbed during the absorb-replan loop.

#### Scenario: Absorb event in JSONL

- **WHEN** `popAllPending` returns non-empty pending messages
- **THEN** a business log entry SHALL be written with `traceEvent: "absorb"`, `absorbRound`, and `absorbedCount` fields BEFORE the messages are appended to context

#### Scenario: Trace builder processes absorb events

- **WHEN** the trace builder encounters a `traceEvent: "absorb"` log entry
- **THEN** it SHALL create an ExecutionStep with `type: "absorb"` containing the absorb metadata (round number, count)

### Requirement: Frontend absorb round display

The Decision Inspector SHALL display absorb rounds as distinct phases within a trace.

#### Scenario: Identify rounds from trace steps

- **WHEN** a trace contains steps with `type: "absorb"`
- **THEN** the frontend SHALL split the trace steps into rounds, where each absorb event marks the boundary between rounds

#### Scenario: Round tabs in Inspector

- **WHEN** a trace has 2+ rounds
- **THEN** the Inspector SHALL display round tabs with labels indicating the absorbed message count for each subsequent round
