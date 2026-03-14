## ADDED Requirements

### Requirement: Absorb pending messages after LLM loop completion
After `RunAgentLoop` returns within `processOneEvent`, the system SHALL atomically check the `SessionWorker.Queue` for pending messages. If pending messages exist, they SHALL be dequeued, formatted, and appended to the current in-memory messages array as user messages, then a new `RunAgentLoop` SHALL be invoked with the updated context.

#### Scenario: Single pending message absorbed after loop
- **WHEN** `RunAgentLoop` completes for message A and one message B exists in `SessionWorker.Queue`
- **THEN** message B is popped from the queue, formatted via `formatUserMessage`, appended to messages, and a new `RunAgentLoop` is invoked with `[...A's full context, assistant reply to A, user B]`

#### Scenario: Multiple pending messages absorbed at once
- **WHEN** `RunAgentLoop` completes and messages B, C, D exist in `SessionWorker.Queue`
- **THEN** all three messages are popped atomically, each formatted and appended in order, and a single new `RunAgentLoop` is invoked with all three as consecutive user messages

#### Scenario: No pending messages
- **WHEN** `RunAgentLoop` completes and `SessionWorker.Queue` is empty
- **THEN** `processOneEvent` exits the absorb loop and proceeds to context saving and compact

#### Scenario: Pending message arrives during re-plan loop
- **WHEN** a new `RunAgentLoop` (re-plan round) is executing and a new message arrives in the queue
- **THEN** the message is caught in the next absorb check after this `RunAgentLoop` completes

### Requirement: FinalText always written to messages
When `RunAgentLoop` returns with a non-empty `FinalText`, the system SHALL unconditionally append an assistant message containing the `FinalText` to the in-memory messages array. This applies regardless of whether pending messages exist, ensuring context completeness for both session persistence and absorb continuity.

#### Scenario: FinalText preserved for absorb
- **WHEN** `RunAgentLoop` returns `FinalText = "好的，我来处理"` and a pending message exists
- **THEN** the messages array contains `[..., {role: "assistant", content: "好的，我来处理"}, {role: "user", content: "<formatted pending message>"}]` before the next `RunAgentLoop` call

#### Scenario: FinalText preserved for context save (no pending)
- **WHEN** `RunAgentLoop` returns `FinalText = "任务完成"` and no pending messages exist
- **THEN** the messages array includes the assistant message before context is saved to session, ensuring the LLM's final reply is part of the persisted history

#### Scenario: Empty FinalText skipped
- **WHEN** `RunAgentLoop` returns `FinalText = ""` (loop ended due to max iterations or cancellation)
- **THEN** no assistant message is appended

### Requirement: Absorb round limit
The system SHALL enforce a maximum number of absorb-and-replan rounds (configurable, default 5). If the limit is reached while pending messages still exist, the remaining messages SHALL be left in the queue for `drainWorker` to handle via separate `processOneEvent` calls.

#### Scenario: Absorb limit reached
- **WHEN** `processOneEvent` has completed 5 absorb rounds and new messages still exist in the queue
- **THEN** `processOneEvent` exits the absorb loop, logs a warning, and proceeds to context saving. Remaining queued messages are processed by `drainWorker` as separate events.

#### Scenario: Under absorb limit
- **WHEN** `processOneEvent` has completed 2 absorb rounds and the queue is empty
- **THEN** `processOneEvent` exits normally after the 2nd round

### Requirement: Atomic queue drain operation
The system SHALL provide a `popAllPending` function that atomically dequeues all pending messages from `SessionWorker.Queue` under `workerMutex` protection. The function returns the dequeued messages or nil if the queue is empty.

#### Scenario: Concurrent enqueue during pop
- **WHEN** `popAllPending` is called while `EnqueueProcessRequest` is adding a new message
- **THEN** mutual exclusion via `workerMutex` ensures no data race; the new message is either included in this pop or left for the next check

#### Scenario: Empty queue pop
- **WHEN** `popAllPending` is called on an empty queue
- **THEN** nil is returned and the queue remains empty

### Requirement: Per-round checkpoint with water level check
After each `RunAgentLoop` round (including re-plan rounds), the system SHALL perform a checkpoint: estimate token count, compact if water level is exceeded (`ShouldCompact` returns true), and save the current messages to `session.Context` via `storage.UpdateSession`. If compact is performed, the compacted messages SHALL replace the in-memory messages for subsequent rounds.

#### Scenario: Water level safe, context saved
- **WHEN** `RunAgentLoop` completes round 1 and `ShouldCompact` returns false
- **THEN** `CompactContext` is NOT called; `UpdateSession(context)` is called with current messages

#### Scenario: Water level exceeded, compact then save
- **WHEN** `RunAgentLoop` completes round 2 and `ShouldCompact` returns true
- **THEN** `CompactContext` is called, messages are replaced with compacted result, `UpdateSession(context)` is called with compacted messages, and the next `RunAgentLoop` (if absorb continues) uses the compacted messages

#### Scenario: Crash recovery after 2 rounds
- **WHEN** `processOneEvent` completes 2 rounds with checkpoints, then crashes during round 3
- **THEN** session.Context contains the state from the end of round 2; round 3 progress is lost but rounds 1-2 are durable

### Requirement: OnNewMessages callback active in all rounds
The `OnNewMessages` callback (which persists tool_use and tool_result to the messages DB table) SHALL remain active during re-plan rounds. Each round's tool interactions are persisted the same way as the initial round.

#### Scenario: Tool use in re-plan round persisted
- **WHEN** during a re-plan round the LLM calls `send_channel_message` and receives a tool result
- **THEN** both the tool_use and tool_result are saved to the messages table via `OnNewMessages`, with the same sessionId and traceId as the initial round

### Requirement: Absorb activity logged at business level
Each absorb event SHALL be logged at business level with the number of absorbed messages and the current absorb round number.

#### Scenario: Absorb logged
- **WHEN** 2 pending messages are absorbed in round 1
- **THEN** a business-level log entry is emitted: "发现新消息，重新规划" with fields `absorbedCount=2, absorbRound=1, messageContents=[summary]`
