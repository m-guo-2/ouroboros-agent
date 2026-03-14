## ADDED Requirements

### Requirement: data_report sub-agent profile
The system SHALL define a `data_report` sub-agent profile in the subagent manager with a dedicated tool set and system prompt optimized for data visualization card generation.

#### Scenario: Main agent dispatches data report task
- **WHEN** the main agent calls `run_subagent_async` with `profile: "data_report"` and a task describing data to visualize
- **THEN** the subagent manager creates a job running the data_report profile with its designated tools and system prompt

#### Scenario: Profile restricts tool access
- **WHEN** a data_report sub-agent job starts
- **THEN** it has access only to `render_card`, `read_file`, and `list_dir` tools (plus the standard `recall_context` and `save_memory`)

### Requirement: data_report system prompt
The system SHALL provide a system prompt for the data_report profile that instructs the LLM to analyze incoming data, select an appropriate card type, prefer pre-built templates, and produce an image URL as the primary output.

#### Scenario: Sub-agent selects template based on data shape
- **WHEN** the sub-agent receives a task with 3 KPI metrics (title, value, trend)
- **THEN** it selects the `kpi` template and calls `render_card` with template mode

#### Scenario: Sub-agent falls back to freeform HTML
- **WHEN** the sub-agent receives data that does not fit any pre-built template
- **THEN** it generates custom HTML and calls `render_card` with freeform mode

#### Scenario: Sub-agent returns structured result
- **WHEN** the sub-agent completes rendering successfully
- **THEN** its result summary includes `imageUrl` (the OSS presigned URL), `cardType` (which template or "freeform"), and a brief text description of the card content

### Requirement: Fallback on rendering failure
The system SHALL ensure the data_report sub-agent returns a usable fallback when rendering fails, so the main agent can still deliver information to the user.

#### Scenario: Rendering fails, sub-agent returns formatted text
- **WHEN** the `render_card` tool returns an error (browser unavailable, timeout, OSS failure)
- **THEN** the sub-agent generates a formatted text version of the data (using markdown tables or structured layout) and returns it with `fallback: true` and `fallbackText` containing the formatted content

#### Scenario: Main agent handles fallback result
- **WHEN** the main agent receives a data_report sub-agent result with `fallback: true`
- **THEN** it sends the `fallbackText` via `send_channel_message` with `messageType: "text"` or `"rich_text"` instead of `"image"`

### Requirement: Main agent trigger guidance
The system SHALL include guidance in the main agent's system prompt that describes when to delegate information to the data_report sub-agent.

#### Scenario: Main agent decides to use card for multi-metric response
- **WHEN** the main agent is about to reply with information containing 3 or more numerical metrics, status comparisons, rankings, or trend data
- **THEN** it considers delegating to the data_report sub-agent instead of sending plain text

#### Scenario: User explicitly requests visual output
- **WHEN** the user says phrases like "出个图", "做个报表", "可视化一下", or "generate a chart"
- **THEN** the main agent delegates to the data_report sub-agent

#### Scenario: Main agent sends image from sub-agent result
- **WHEN** the data_report sub-agent returns successfully with an `imageUrl`
- **THEN** the main agent calls `send_channel_message` with `messageType: "image"` and `content` set to the image URL, optionally preceded or followed by a brief text summary
