## ADDED Requirements

### Requirement: Agent receives conversationType on every incoming message

The agent `IncomingMessage` struct SHALL include a `ConversationType` field (`string`, JSON tag `conversationType`). Channel-qiwei already sends this field; the agent SHALL deserialize it.

Valid values: `"p2p"` (private chat), `"group"` (group chat).

#### Scenario: Group message carries conversationType=group

- **WHEN** a message arrives from a group chat (callback `fromRoomId` is non-empty and not `"0"`)
- **THEN** the incoming message to agent SHALL have `conversationType` = `"group"`

#### Scenario: Private message carries conversationType=p2p

- **WHEN** a message arrives from a private chat (callback `fromRoomId` is empty or `"0"`)
- **THEN** the incoming message to agent SHALL have `conversationType` = `"p2p"`

---

### Requirement: Text messages carry atList in channelMeta

When a text message (msgType 0, 1, 2) contains `atList` in `msgData`, channel-qiwei SHALL extract it and include it in the `channelMeta` of the incoming message forwarded to agent.

The `channelMeta.atList` SHALL be an array of objects, each containing `userId` (string) and `nickname` (string).

#### Scenario: Text message with @mentions

- **WHEN** a text message callback has `msgData.atList` = `[{"userId":"168885...","nickname":"张三"},{"userId":"788130...","nickname":"Bot"}]`
- **THEN** the incoming message `channelMeta.atList` SHALL be `[{"userId":"168885...","nickname":"张三"},{"userId":"788130...","nickname":"Bot"}]`

#### Scenario: Text message without @mentions

- **WHEN** a text message callback has `msgData.atList` as null or empty
- **THEN** the incoming message `channelMeta` SHALL NOT contain an `atList` key (or it SHALL be omitted)

---

### Requirement: Text messages carry mentionedSelf in channelMeta

Channel-qiwei SHALL compare the `atList` user IDs against the bot's own userId. If the bot is mentioned, `channelMeta.mentionedSelf` SHALL be `true`.

The bot's own userId SHALL be obtained at startup via `/user/getProfile` or from the callback's top-level `userId` field as fallback.

#### Scenario: Bot is @mentioned in group

- **WHEN** a text message callback has `msgData.atList` containing an entry whose `userId` matches the bot's own userId
- **THEN** the incoming message `channelMeta.mentionedSelf` SHALL be `true`

#### Scenario: Bot is not @mentioned

- **WHEN** a text message callback has `msgData.atList` that does NOT contain the bot's own userId, or `atList` is empty/null
- **THEN** the incoming message `channelMeta` SHALL NOT contain `mentionedSelf`, or it SHALL be `false`

#### Scenario: Self userId not yet known

- **WHEN** channel-qiwei has not yet resolved its own userId (e.g. startup race)
- **THEN** the incoming message `channelMeta` SHALL NOT contain `mentionedSelf` (omit rather than guess)
