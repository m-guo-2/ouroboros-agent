## ADDED Requirements

### Requirement: Web research subagent profile availability
The system SHALL support a built-in subagent profile named `web_research` for delegated external research tasks, and that profile SHALL operate as a constrained wrapper around `tavily_search` rather than as a separate Tavily integration path.

#### Scenario: Starting a web research subagent
- **WHEN** the main agent starts a subagent with profile `web_research`
- **THEN** the subagent manager MUST accept the profile and create a subagent job for that task

#### Scenario: Research profile uses builtin Tavily path
- **WHEN** a `web_research` subagent performs external search
- **THEN** it MUST rely on the built-in `tavily_search` capability instead of a separate MCP or skill-based Tavily path

#### Scenario: Invalid research profile name
- **WHEN** a caller uses an unsupported research-oriented profile name
- **THEN** the subagent manager MUST reject the request with a profile validation error

### Requirement: Web research subagent tool restrictions
The system SHALL restrict the `web_research` subagent profile to a minimal tool set intended for external research.

#### Scenario: Allowed research tools
- **WHEN** a `web_research` subagent is initialized
- **THEN** its allowed tool set MUST include `tavily_search` and any explicitly approved low-risk context tools required by the profile

#### Scenario: Disallowed high-privilege tools
- **WHEN** a `web_research` subagent is initialized
- **THEN** it MUST NOT receive high-privilege tools such as shell execution or file write access

### Requirement: Web research subagent prompt contract
The system SHALL provide a profile-specific system prompt that instructs the subagent to prioritize Tavily-based web retrieval, handle large result sets conservatively, and return a concise summary for the parent agent.

#### Scenario: Research-first prompt behavior
- **WHEN** the runtime builds the system prompt for profile `web_research`
- **THEN** the prompt MUST instruct the subagent to use Tavily for external information gathering before producing its final summary

#### Scenario: Parent-facing output
- **WHEN** a `web_research` subagent finishes successfully
- **THEN** its final output MUST be a natural-language summary intended for the parent agent rather than a direct end-user reply

### Requirement: Web research subagent iteratively narrows large result sets
The system SHALL guide the `web_research` subagent to handle large search result sets through iterative retrieval rather than one-shot bulk ingestion.

#### Scenario: Initial search returns too many possible sources
- **WHEN** a `web_research` subagent detects that the initial Tavily search has many possible matches or a truncated result set
- **THEN** it MUST prefer refining the query or running another focused search before writing its final summary

#### Scenario: Research summary avoids bulk result dumping
- **WHEN** a `web_research` subagent prepares its final output
- **THEN** it MUST summarize the most relevant findings instead of dumping the full search result list into the parent-facing response

### Requirement: Web research subagent degraded behavior
The system SHALL fail gracefully when Tavily is unavailable to the `web_research` profile.

#### Scenario: Tavily unavailable during research task
- **WHEN** a `web_research` subagent attempts external research while Tavily is disabled or misconfigured
- **THEN** the subagent MUST report the blocking reason in its result or failure state so the parent agent can decide the next action
