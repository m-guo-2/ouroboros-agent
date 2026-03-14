## ADDED Requirements

### Requirement: Adapter SHALL route callback messages by cmd field

adapter 处理 QiWei 回调时 SHALL 先按 `cmd` 字段分流，不再假定所有消息都是 cmd=15000。

#### Scenario: 收到 cmd=15000 的普通消息
- **WHEN** QiWei 回调 payload 中 data[].cmd=15000
- **THEN** adapter 按现有普通消息逻辑处理（msgType 映射 → 媒体下载 → 转发 agent）

#### Scenario: 收到 cmd=15500 的系统消息
- **WHEN** QiWei 回调 payload 中 data[].cmd=15500
- **THEN** adapter 进入系统消息处理分支，按 msgType 进一步路由

#### Scenario: 收到 cmd=11016 的状态消息
- **WHEN** QiWei 回调 payload 中 data[].cmd=11016
- **THEN** adapter 记录日志，不转发给 agent（行为不变）

#### Scenario: 收到 cmd=20000 的异步消息
- **WHEN** QiWei 回调 payload 中 data[].cmd=20000
- **THEN** adapter 记录日志，不转发给 agent（行为不变）

### Requirement: qiweiCallbackMessage SHALL include cmd field

`qiweiCallbackMessage` 结构体 SHALL 包含 `Cmd` 字段，`decodeOneMessage` SHALL 从回调 payload 中解析该字段。

#### Scenario: 解析包含 cmd 的回调消息
- **WHEN** QiWei 回调 payload 的 data 数组中每个元素包含 "cmd" 字段
- **THEN** `decodeOneMessage` 将 cmd 值解析到 `qiweiCallbackMessage.Cmd` 中

#### Scenario: 回调消息缺少 cmd 字段
- **WHEN** 回调 payload 中某条消息不包含 "cmd" 字段
- **THEN** `Cmd` 默认为 15000（兼容旧行为，按普通消息处理）

### Requirement: New group member event SHALL be forwarded to agent

cmd=15500 且 msgType=1002 的新成员入群事件 SHALL 被构造为 system 类型消息转发给 agent。

#### Scenario: 新成员入群
- **WHEN** 系统消息 cmd=15500, msgType=1002，msgData.changedMemberList 非空
- **THEN** adapter 构造 messageType="system" 的 incomingMessage，channelConversationID 为 fromRoomId（群 ID），content 格式为 `"[群事件] 新成员加入了群聊"`

#### Scenario: 新成员入群但 fromRoomId 为空
- **WHEN** 系统消息 cmd=15500, msgType=1002，但 fromRoomId 为空
- **THEN** adapter 记录 warn 日志，跳过该事件

### Requirement: Friend request events SHALL be logged

cmd=15500 且 msgType=2357 或 2132 的好友申请事件 SHALL 被记录到日志，暂不推送给 agent。

#### Scenario: 收到好友申请 (msgType=2357)
- **WHEN** 系统消息 cmd=15500, msgType=2357，msgData 中包含 contactNickname 和 contactId
- **THEN** adapter 记录 info 日志，包含申请人昵称和 contactId，不转发给 agent

#### Scenario: 收到好友申请 (msgType=2132)
- **WHEN** 系统消息 cmd=15500, msgType=2132
- **THEN** adapter 记录 info 日志，不转发给 agent

### Requirement: Other system events SHALL be silently logged

cmd=15500 中除 1002/2357/2132 之外的 msgType SHALL 仅记录日志，不推送 agent。

#### Scenario: 群名变更 (1001)
- **WHEN** 系统消息 cmd=15500, msgType=1001
- **THEN** adapter 记录 info 日志，不转发给 agent

#### Scenario: 成员退群 (1003/1005)
- **WHEN** 系统消息 cmd=15500, msgType=1003 或 1005
- **THEN** adapter 记录 info 日志，不转发给 agent

#### Scenario: 联系人变动 (2131/2188)
- **WHEN** 系统消息 cmd=15500, msgType=2131 或 2188
- **THEN** adapter 记录 info 日志，不转发给 agent
