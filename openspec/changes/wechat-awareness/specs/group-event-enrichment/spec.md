## ADDED Requirements

### Requirement: Group events carry affected member IDs

When channel-qiwei forwards a group event (`member_joined`, `member_removed`, `member_quit`) to agent, the `GroupEvent.Payload` SHALL include a `memberIds` field — an array of userId strings extracted from the callback's `msgData.changedMemberList`.

The `changedMemberList` value is base64-encoded. After decoding, it contains semicolon-separated userId strings.

#### Scenario: Member joined with changedMemberList

- **WHEN** a `member_joined` (msgType=1002) callback has `msgData.changedMemberList` = base64("1688855989642487;1688857631651804")
- **THEN** the `GroupEvent.Payload.memberIds` sent to agent SHALL be `["1688855989642487", "1688857631651804"]`

#### Scenario: Member removed with changedMemberList

- **WHEN** a `member_removed` (msgType=1003) callback has `msgData.changedMemberList` = base64("1688855989642487")
- **THEN** the `GroupEvent.Payload.memberIds` sent to agent SHALL be `["1688855989642487"]`

#### Scenario: changedMemberList is empty or decode fails

- **WHEN** a group event callback has `msgData.changedMemberList` as empty string, null, or fails base64 decoding
- **THEN** the `GroupEvent.Payload.memberIds` SHALL be an empty array `[]`; the event SHALL still be delivered

---

### Requirement: Group events carry operator ID

When channel-qiwei forwards a group event, the `GroupEvent.Payload` SHALL include an `operatorId` field — a string extracted from the callback's `senderId`.

#### Scenario: Member removed by group owner

- **WHEN** a `member_removed` (msgType=1003) callback has `senderId` = `"168885763165180"`
- **THEN** the `GroupEvent.Payload.operatorId` sent to agent SHALL be `"168885763165180"`

#### Scenario: senderId is zero or empty

- **WHEN** a group event callback has `senderId` as `"0"` or empty
- **THEN** the `GroupEvent.Payload.operatorId` SHALL be omitted or empty string

---

### Requirement: new_contact is a valid group event type on agent side

The agent SHALL accept `"new_contact"` as a valid event type in `validGroupEventTypes`. This allows the auto-accept-friend flow to reuse the group-event endpoint.

#### Scenario: new_contact event is accepted by agent

- **WHEN** channel-qiwei sends a `GroupEvent` with `eventType` = `"new_contact"` to `/api/channels/group-event`
- **THEN** the agent SHALL accept the event (not reject with "invalid event type")
