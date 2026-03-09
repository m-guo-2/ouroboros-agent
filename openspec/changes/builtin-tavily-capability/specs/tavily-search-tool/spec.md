## ADDED Requirements

### Requirement: Builtin Tavily search tool availability
The system SHALL expose a builtin tool named `tavily_search` when Tavily integration is enabled in system settings, and that tool SHALL be the canonical built-in Tavily access path for agent web search.

#### Scenario: Tavily tool is registered when enabled
- **WHEN** the agent runtime starts with Tavily enabled
- **THEN** the tool registry exposes `tavily_search` as a builtin tool available to the main agent

#### Scenario: Tavily tool is not usable when disabled
- **WHEN** Tavily integration is disabled by configuration
- **THEN** calls to `tavily_search` MUST fail with an actionable error indicating that Tavily is disabled

### Requirement: Tavily tool configuration validation
The system SHALL validate Tavily credentials and endpoint configuration before making an outbound request.

#### Scenario: Missing API key
- **WHEN** an agent calls `tavily_search` without a configured Tavily API key
- **THEN** the tool MUST return a clear configuration error and MUST NOT send an outbound request

#### Scenario: Default base URL
- **WHEN** an agent calls `tavily_search` and no custom Tavily base URL is configured
- **THEN** the tool MUST use the system default Tavily API base URL

### Requirement: Tavily tool returns normalized search results
The system SHALL normalize Tavily responses into a stable structure containing the query, a summary or answer field when available, a result list with source URLs, and explicit truncation metadata.

#### Scenario: Successful search result normalization
- **WHEN** Tavily returns a successful search response
- **THEN** `tavily_search` MUST return the original query and a `results` list containing normalized source entries with URL information

#### Scenario: Optional answer field passthrough
- **WHEN** Tavily returns an answer or summary field
- **THEN** `tavily_search` MUST include that answer content in the normalized tool output

#### Scenario: Normalized metadata shape
- **WHEN** `tavily_search` returns a successful normalized response
- **THEN** the output MUST include `query`, `results`, `total_results`, `returned_results`, and `truncated`

### Requirement: Tavily tool caps high-volume search results
The system SHALL cap the number and size of Tavily search results before returning them to the agent context.

#### Scenario: Result count exceeds return budget
- **WHEN** Tavily returns more results than the configured return budget
- **THEN** `tavily_search` MUST return only the top in-budget results and MUST mark the response with `truncated = true`

#### Scenario: Caller requests too many results
- **WHEN** a caller passes a `max_results` value above the platform limit
- **THEN** `tavily_search` MUST clamp the request to the configured maximum instead of returning an unbounded result set

#### Scenario: Large result set metadata is preserved
- **WHEN** `tavily_search` truncates the result list
- **THEN** the normalized output MUST include both `total_results` and `returned_results` so the caller can detect that additional results exist

#### Scenario: Individual snippets are length-limited
- **WHEN** a Tavily result contains a long content field
- **THEN** `tavily_search` MUST shorten that content into a bounded snippet rather than returning the full raw text

### Requirement: Tavily tool failure semantics and observability
The system SHALL surface Tavily request failures as structured tool errors and record lightweight execution metadata for tracing.

#### Scenario: Upstream request failure
- **WHEN** the Tavily API returns a non-success response or times out
- **THEN** `tavily_search` MUST return a structured failure message that identifies the request as an upstream Tavily error

#### Scenario: Trace metadata capture
- **WHEN** `tavily_search` completes or fails
- **THEN** the runtime MUST record lightweight metadata including the query, total result count, returned result count, truncation status, and whether the request succeeded without logging full raw page content
