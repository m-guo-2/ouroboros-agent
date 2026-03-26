## ADDED Requirements

### Requirement: Handle group name change event (msgType 1001)
系统 SHALL 识别 msgType=1001 的回调消息（无论 cmd=15000 或 cmd=15500），将其作为群名变更事件转发给 agent。

#### Scenario: Group name change arrives with cmd=15000
- **WHEN** 收到 cmd=15000, msgType=1001, fromRoomId 非空的回调消息
- **THEN** 系统构造 messageType="system", content="[群事件] 群名称已变更" 的消息，channelConversationId 为 fromRoomId，转发给 agent server

#### Scenario: Group name change without agent enabled
- **WHEN** 收到 msgType=1001 但 AgentEnabled=false
- **THEN** 系统记录日志但不转发

### Requirement: Handle member removed event (msgType 1003)
系统 SHALL 识别 msgType=1003 的回调消息，将其作为成员被移除事件转发给 agent。

#### Scenario: Member removed from group
- **WHEN** 收到 cmd=15000, msgType=1003, fromRoomId 非空的回调消息
- **THEN** 系统构造 messageType="system", content="[群事件] 成员被移出群聊" 的消息，转发给 agent server

#### Scenario: Member removed without roomId
- **WHEN** 收到 msgType=1003 但 fromRoomId 为空或 "0"
- **THEN** 系统记录警告日志，不转发

### Requirement: Handle member quit event (msgType 1005)
系统 SHALL 识别 msgType=1005 的回调消息，将其作为成员主动退群事件转发给 agent。

#### Scenario: Member voluntarily leaves group
- **WHEN** 收到 cmd=15000, msgType=1005, fromRoomId 非空的回调消息
- **THEN** 系统构造 messageType="system", content="[群事件] 成员退出群聊" 的消息，转发给 agent server

### Requirement: Handle group dissolved event (msgType 1023)
系统 SHALL 识别 msgType=1023 的回调消息，将其作为群解散事件转发给 agent。

#### Scenario: Group is dissolved
- **WHEN** 收到 cmd=15000, msgType=1023, fromRoomId 非空的回调消息
- **THEN** 系统构造 messageType="system", content="[群事件] 群聊已解散" 的消息，转发给 agent server

### Requirement: Dual-path compatibility for member joined (msgType 1002)
系统 SHALL 在 handleNormalMessage（cmd=15000）路径中也能处理 msgType=1002，与现有 handleSystemEvent（cmd=15500）路径形成兼容。

#### Scenario: Member joined arrives with cmd=15000
- **WHEN** 收到 cmd=15000, msgType=1002, fromRoomId 非空的回调消息
- **THEN** 系统构造 messageType="system", content="[群事件] 新成员加入了群聊" 的消息，转发给 agent server

#### Scenario: Member joined arrives with cmd=15500 (existing behavior)
- **WHEN** 收到 cmd=15500, msgType=1002 的回调消息
- **THEN** 行为与当前 handleGroupMemberJoined 一致，不受本次变更影响
