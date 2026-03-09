## ADDED Requirements

### Requirement: Stats summary bar

The Decision Inspector SHALL display a stats summary bar at the top for the selected exchange's trace.

#### Scenario: Completed trace stats

- **WHEN** a completed trace is selected
- **THEN** the stats bar SHALL display: total duration, total input tokens, total output tokens, total cost (USD), LLM call count, iteration count

#### Scenario: Running trace stats

- **WHEN** a running trace is selected
- **THEN** the stats bar SHALL display live-updating elapsed time and token counts accumulated so far

### Requirement: Absorb round tabs

When a trace contains multiple absorb rounds, the Inspector SHALL display tabs for each round.

#### Scenario: Single round (no absorb)

- **WHEN** a trace has no absorb events (single execution round)
- **THEN** no tabs SHALL be shown; the round content SHALL be displayed directly

#### Scenario: Multiple rounds

- **WHEN** a trace has absorb events indicating multiple rounds
- **THEN** tabs SHALL appear labeled "Round 1", "Round 2 (+N 条新消息)", etc.
- **AND** each tab SHALL show that round's execution detail

### Requirement: Thinking blocks display

LLM thinking blocks SHALL be displayed prominently in the Inspector, defaulting to expanded.

#### Scenario: Thinking visible by default

- **WHEN** a round contains thinking steps
- **THEN** thinking content SHALL be displayed expanded (not collapsed behind a click)

#### Scenario: System vs model thinking

- **WHEN** a thinking step has source "system"
- **THEN** it SHALL be visually distinguished from model thinking (different icon/color)

### Requirement: Model output display

The Inspector SHALL display the model's text output for each round.

#### Scenario: Model output from LLM I/O

- **WHEN** a round has an llmIORef
- **THEN** the model's text output SHALL be extracted from the LLM I/O response and displayed

#### Scenario: Model output without explicit text

- **WHEN** a round's LLM response contains only tool_use blocks (no text)
- **THEN** a label SHALL indicate "本轮以工具调用为主，无文本输出"

### Requirement: Tool execution display

Tool calls SHALL be displayed as individual cards with input and result.

#### Scenario: Successful tool call

- **WHEN** a tool call completed successfully
- **THEN** a card SHALL show: tool name, duration, input (pretty-printed JSON), result (pretty-printed JSON), with a success indicator

#### Scenario: Failed tool call

- **WHEN** a tool call failed
- **THEN** a card SHALL show: tool name, input, error message, with a failure indicator

#### Scenario: Tool input/result expandable

- **WHEN** tool input or result content is large (>500 chars)
- **THEN** it SHALL be truncated with an expand button and a "全文" link to open in a new tab

### Requirement: Compaction detail in Inspector

When a compaction occurred during the selected exchange's processing, the Inspector SHALL display compaction details.

#### Scenario: Compaction within a round

- **WHEN** a compaction event (step type "compact") exists in the trace
- **THEN** the Inspector SHALL display: token count before/after, archived message count, and summary text

### Requirement: Raw LLM I/O access

The Inspector SHALL provide access to the full raw LLM request/response for each LLM call.

#### Scenario: Open raw LLM I/O

- **WHEN** the user clicks the "I/O" button on an LLM call
- **THEN** a modal SHALL display the full request and response JSON with syntax highlighting and expand/collapse
