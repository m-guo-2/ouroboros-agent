## ADDED Requirements

### Requirement: Render HTML to PNG image
The system SHALL provide a card rendering capability that accepts an HTML string, renders it in a headless Chrome browser via Rod, captures a PNG screenshot, and returns the image bytes.

#### Scenario: Render valid HTML to PNG
- **WHEN** a caller provides a non-empty HTML string to the rendering capability
- **THEN** the system renders the HTML in a headless Chrome page, captures a full-page PNG screenshot, and returns the image bytes with dimensions (width, height)

#### Scenario: Render HTML with custom viewport width
- **WHEN** a caller provides an HTML string and specifies a viewport width (e.g., 600px)
- **THEN** the system renders the page at the specified width and captures the screenshot accordingly

#### Scenario: Reject empty HTML input
- **WHEN** a caller provides an empty or whitespace-only HTML string
- **THEN** the system returns a validation error without launching the browser

#### Scenario: Handle rendering timeout
- **WHEN** the HTML page does not finish loading within the configured timeout (default 30 seconds)
- **THEN** the system cancels the rendering and returns a timeout error

### Requirement: Upload rendered image to OSS
The system SHALL upload the rendered PNG image to the shared OSS storage and return a presigned URL that is accessible for at least 7 days.

#### Scenario: Upload PNG and return presigned URL
- **WHEN** a PNG image has been rendered successfully
- **THEN** the system uploads it to OSS under a generated key with content type `image/png`, then returns a presigned URL with a minimum expiry of 7 days

#### Scenario: Handle OSS upload failure
- **WHEN** the OSS upload fails (network error, auth error, etc.)
- **THEN** the system returns an OSS-related error and does not return a partial URL

### Requirement: Template-based rendering
The system SHALL support rendering from pre-built HTML templates by accepting a template name and a data object, then populating the template with the data before rendering.

#### Scenario: Render using a named template with data
- **WHEN** a caller specifies a template name (e.g., `kpi`) and provides a JSON data object
- **THEN** the system loads the corresponding HTML template, fills in the data placeholders using Go text/template, and renders the populated HTML to PNG

#### Scenario: Reject unknown template name
- **WHEN** a caller specifies a template name that does not exist in the templates directory
- **THEN** the system returns an error listing the available template names

#### Scenario: Render with freeform HTML when no template specified
- **WHEN** a caller provides an `html` field without specifying a `template` name
- **THEN** the system renders the provided HTML directly without template processing

### Requirement: Browser instance management
The system SHALL manage headless Chrome browser instances efficiently, reusing instances across render calls and cleaning up idle instances.

#### Scenario: Reuse browser across sequential renders
- **WHEN** multiple render requests arrive within a short time window
- **THEN** the system reuses the same browser instance rather than launching a new one for each request

#### Scenario: Clean up idle browser
- **WHEN** no render requests have arrived for a configurable idle duration (default 5 minutes)
- **THEN** the system closes the idle browser instance to free resources

#### Scenario: Limit concurrent renders
- **WHEN** the number of concurrent render requests exceeds the configured maximum (default 3)
- **THEN** additional requests are queued or rejected with a capacity error, rather than launching unbounded browser instances

### Requirement: render_card tool interface
The system SHALL expose card rendering as a registered tool named `render_card` that the data_report sub-agent can invoke.

#### Scenario: Invoke render_card with template mode
- **WHEN** the sub-agent calls `render_card` with `{"template": "kpi", "data": {"title": "Revenue", "value": "$1.2M", "trend": "+12%"}}`
- **THEN** the tool renders the KPI template with the provided data and returns `{"imageUrl": "<presigned-url>", "width": 600, "height": 400}`

#### Scenario: Invoke render_card with freeform HTML
- **WHEN** the sub-agent calls `render_card` with `{"html": "<html>...<\/html>"}`
- **THEN** the tool renders the HTML directly and returns `{"imageUrl": "<presigned-url>", "width": ..., "height": ...}`

#### Scenario: render_card reports failure with actionable message
- **WHEN** the rendering or upload fails for any reason
- **THEN** the tool returns an error message that includes the failure category (render_timeout, browser_unavailable, oss_upload_failed, invalid_template) so the sub-agent can decide on fallback

### Requirement: Pre-built card templates
The system SHALL include a set of pre-built HTML card templates stored in `agent/data/card-templates/`, each as a standalone HTML file with Go text/template placeholders.

#### Scenario: KPI template renders large numbers with trend indicator
- **WHEN** the `kpi` template receives data with `title`, `value`, `trend`, and optional `comparison`
- **THEN** it renders a card with the value in large typography, a color-coded trend indicator (green for positive, red for negative), and the comparison baseline

#### Scenario: Table template renders structured data rows
- **WHEN** the `table` template receives data with `title`, `headers`, and `rows`
- **THEN** it renders a card with a styled table with alternating row colors and header highlighting

#### Scenario: Status-board template renders multi-item status
- **WHEN** the `status-board` template receives data with `title` and `items` (each having `name` and `status` of ok/warning/error)
- **THEN** it renders a card with color-coded status indicators (green/yellow/red) for each item

#### Scenario: Ranking template renders ordered items with bars
- **WHEN** the `ranking` template receives data with `title` and `items` (each having `name` and `value`)
- **THEN** it renders a card with horizontal bar chart entries, ordered by value, with rank numbers

#### Scenario: Timeline template renders event sequence
- **WHEN** the `timeline` template receives data with `title` and `events` (each having `time` and `description`)
- **THEN** it renders a vertical timeline card with nodes and connecting lines

#### Scenario: Summary template renders key-value information
- **WHEN** the `summary` template receives data with `title` and `items` (each having `label` and `value`)
- **THEN** it renders a clean key-value layout card suitable for general information display
