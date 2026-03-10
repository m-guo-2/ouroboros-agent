# QiWe Media Pipeline Spec

## ADDED Requirements

### Requirement: Media message classification

The system SHALL classify QiWe media messages into a stable semantic model before any attachment download or analysis begins. The classification result MUST include the original `msgType`, the message source (`qw` for enterprise WeChat or `gw` for personal WeChat), and the media kind (`image`, `file`, `voice`, or `video`).

#### Scenario: Classify enterprise media message

- **WHEN** the system receives a callback or history message whose `msgType` maps to an enterprise WeChat media type
- **THEN** the system classifies the message with `source=qw` and the correct media kind before planning any download

#### Scenario: Classify personal media message

- **WHEN** the system receives a callback or history message whose `msgType` maps to a personal WeChat media type
- **THEN** the system classifies the message with `source=gw` and the correct media kind before planning any download

#### Scenario: Reject unsupported media type explicitly

- **WHEN** the system receives a media-like message whose `msgType` is not covered by the classifier
- **THEN** the system records a structured unsupported-type result instead of silently guessing a download strategy

### Requirement: Attachment normalization

The system SHALL normalize raw QiWe media payloads into a unified attachment descriptor before invoking any download API. The descriptor MUST consolidate naming variants for the same field, preserve the original raw payload, and expose the fields required by downstream download planning.

#### Scenario: Normalize key naming variants

- **WHEN** a raw payload contains field variants such as `fileAesKey` and `fileAeskey`, or `fileAuthKey` and `fileAuthkey`
- **THEN** the normalized descriptor exposes one canonical value for each field and preserves the source payload for diagnostics

#### Scenario: Normalize direct media URLs by variant

- **WHEN** a raw payload contains multiple media URLs such as `fileBigHttpUrl`, `fileMiddleHttpUrl`, or `fileThumbHttpUrl`
- **THEN** the normalized descriptor records the available variants and identifies the preferred URL candidate without downloading yet

### Requirement: Source-specific download planning

The system SHALL determine the download strategy from the normalized descriptor using source-specific rules instead of fallback guessing. Enterprise WeChat media and personal WeChat media MUST use different planning rules and MUST NOT share the same implicit fallback chain.

#### Scenario: Plan enterprise media download

- **WHEN** a normalized descriptor represents enterprise WeChat media with the fields required by the enterprise download contract
- **THEN** the planner selects an enterprise strategy that uses `/cloud/wxWorkDownload` or `/cloud/cdnWxDownload` as applicable

#### Scenario: Plan personal media download

- **WHEN** a normalized descriptor represents personal WeChat media with the fields required by the personal download contract
- **THEN** the planner selects a personal strategy that uses `/cloud/wxDownload` with the required source-specific parameters

#### Scenario: Reject incomplete contract before execution

- **WHEN** the normalized descriptor does not contain the fields required by the selected source-specific contract
- **THEN** the planner returns a structured planning failure and the executor is not invoked

### Requirement: Voice messages are converted to text before agent handoff

The system SHALL treat voice messages as a special entry-point flow. Voice messages MUST be transcribed inside `channel-qiwei`, and the agent MUST receive transcription text instead of a voice attachment processing state.

#### Scenario: Forward voice transcription as text

- **WHEN** the system successfully transcribes a voice message
- **THEN** the agent-facing payload contains the transcription text and does not require the agent to fetch or inspect a voice file

#### Scenario: Hide voice pipeline internals from agent

- **WHEN** the system cannot fully complete voice resource preparation or transcription
- **THEN** the agent-facing payload uses a unified degraded text result and does not expose download or transcription error internals

### Requirement: File-like media is prepared but not pre-analyzed before agent handoff

The system SHALL prepare image, file, and video resources inside `channel-qiwei` before handing the message to the agent. The system MUST materialize those resources into stable URIs, but MUST NOT pre-run OCR, image understanding, document summarization, or video understanding as part of normal callback forwarding.

#### Scenario: Materialize file-like media before forwarding

- **WHEN** the system successfully processes an image, file, or video message
- **THEN** it stores the resource in OSS or another stable resource location before forwarding the message to the agent

#### Scenario: Do not pre-analyze image content during callback forwarding

- **WHEN** callback handling prepares an image attachment for the agent
- **THEN** the forwarded payload contains the prepared attachment reference without embedding precomputed OCR or visual summary by default

### Requirement: Agent-facing channel payload carries structured attachments

The system SHALL represent non-voice media for the agent as structured attachments rather than only as resource links embedded in the message text. Each forwarded attachment MUST carry a stable identifier plus the minimal actionable resource fields the runtime needs for later analysis.

#### Scenario: Forward image as structured attachment

- **WHEN** callback handling forwards a prepared image message to the agent
- **THEN** the payload includes an attachment object with a stable `attachmentId`, attachment kind, and prepared resource URI

#### Scenario: Hide internal source and download details from attachment payload

- **WHEN** the system forwards a prepared file-like attachment to the agent
- **THEN** the payload does not expose `qw/gw` source, selected download strategy, retry state, or low-level contract fields

### Requirement: Agent can inspect attachments on demand

The system SHALL expose a unified attachment inspection capability so the agent can explicitly analyze a prepared attachment when the current turn requires attachment content understanding.

#### Scenario: Inspect image attachment on demand

- **WHEN** the agent needs to answer a question about the content of a prepared image attachment
- **THEN** it can invoke the attachment inspection capability using the forwarded attachment identifier instead of reconstructing a raw resource URI from message text

#### Scenario: Inspect document attachment on demand

- **WHEN** the agent needs to read or summarize a prepared file attachment
- **THEN** it can invoke the attachment inspection capability with a task appropriate for text extraction or document understanding

#### Scenario: Return structured inspection failure

- **WHEN** attachment inspection cannot complete because of provider configuration, download failure, unsupported format, or timeout
- **THEN** the inspection capability returns a stable structured failure result instead of only a provider-specific raw error

### Requirement: Runtime enforces attachment analysis when needed

The runtime SHALL not rely solely on prompt wording to trigger attachment analysis. If the current user turn depends on attachment content that has not yet been analyzed, the runtime MUST require the agent to use the attachment inspection capability before finalizing a content-dependent answer.

#### Scenario: User asks about image content

- **WHEN** the current turn contains a prepared image attachment and the user asks what is shown in the image
- **THEN** the runtime requires attachment inspection before accepting a final answer about the image content

#### Scenario: User asks unrelated question while attachment exists

- **WHEN** the current turn contains a prepared attachment but the user asks a question that can be answered without inspecting that attachment
- **THEN** the runtime does not force unnecessary attachment inspection

### Requirement: Shared media pipeline across entry points

The system SHALL use the same media pipeline for webhook callback preparation and explicit attachment parsing. Both entry points MUST share classification, normalization, planning, download, and materialization rules even if their upstream consumer contracts differ.

#### Scenario: Callback and parse path agree on media semantics

- **WHEN** the same raw media payload is processed once by callback handling and once by an explicit parse/inspect flow
- **THEN** both entry points produce the same source classification, media kind, and download strategy selection

### Requirement: Structured media observability

The system SHALL emit structured diagnostics for each media processing attempt, including classification result, selected strategy, required-field validation, execution method, and failure stage when applicable.

#### Scenario: Record successful media processing

- **WHEN** a media attachment is classified, planned, downloaded, and materialized successfully
- **THEN** the system records structured diagnostics showing the selected strategy and final materialization state

#### Scenario: Record failed media processing

- **WHEN** media processing fails during classification, normalization, planning, download, materialization, or voice transcription
- **THEN** the system records the failure stage and reason in a structured form that distinguishes contract errors from transport or provider errors

### Requirement: Regression coverage for QiWe media families and attachment handoff

The system SHALL provide regression coverage for enterprise and personal WeChat media families so that supported message types, field normalization, source-specific download planning, voice handoff, and structured attachment forwarding remain stable over time.

#### Scenario: Cover enterprise and personal image/file/video/voice samples

- **WHEN** the media pipeline test suite runs
- **THEN** it verifies representative samples for enterprise and personal media families, including image, file, voice, and video where supported

#### Scenario: Detect attachment handoff regression

- **WHEN** a code change causes image, file, or video messages to stop producing stable attachment identifiers or prepared resource references for the agent
- **THEN** the regression suite fails and reports the handoff regression explicitly
