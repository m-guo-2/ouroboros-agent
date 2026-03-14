## ADDED Requirements

### Requirement: Adapter SHALL handle msgType 0 as text

msgType=0 的回调消息 SHALL 被收敛为 messageType="text"，与 msgType=1/2 行为一致。

#### Scenario: 收到 msgType=0 的文本消息
- **WHEN** QiWei 回调 payload 包含 msgType=0 且 msgData.content 非空
- **THEN** adapter 构造 messageType="text" 的 incomingMessage，content 为 msgData.content 的值，成功转发给 agent

#### Scenario: msgType=0 且 content 为空
- **WHEN** QiWei 回调 payload 包含 msgType=0 但 msgData.content 为空字符串
- **THEN** adapter 记录 warn 日志并跳过该消息，不转发给 agent

### Requirement: Adapter SHALL handle msgType 7 as image

msgType=7 的企微图片消息 SHALL 走现有 image 处理流程（下载 → OSS → 附件）。

#### Scenario: 收到 msgType=7 的企微图片
- **WHEN** QiWei 回调 payload 包含 msgType=7 且 msgData 含 fileId/fileAeskey
- **THEN** adapter 按 QW image 流程下载并上传至 OSS，构造 messageType="image" 的 incomingMessage，附件中包含 resourceUri

### Requirement: Adapter SHALL receive sticker/GIF messages as image

msgType=29 (企微 GIF) 和 msgType=104 (个微 GIF) SHALL 被当作 image 类型处理：下载表情图片 → 上传 OSS → 作为 image 附件转发给 agent。

#### Scenario: 收到企微 GIF 表情 (msgType=29)
- **WHEN** QiWei 回调 payload 包含 msgType=29 且 msgData 含 fileHttpUrl 或 fileMd5
- **THEN** adapter 下载表情图片，上传至 OSS，构造 messageType="sticker" 的 incomingMessage，content 为 "[表情]"，附件 kind="image" 包含 resourceUri

#### Scenario: 收到个微 GIF 表情 (msgType=104)
- **WHEN** QiWei 回调 payload 包含 msgType=104 且 msgData 含 fileHttpUrl
- **THEN** adapter 行为与 msgType=29 相同

#### Scenario: 表情图片下载失败
- **WHEN** 表情图片下载或 OSS 上传失败
- **THEN** adapter 构造 messageType="sticker" 的 incomingMessage，content 为 "[表情]"，无附件，仍转发给 agent

### Requirement: Adapter SHALL receive link messages as text

msgType=13 的链接消息 SHALL 被收敛为人类可读的文本。

#### Scenario: 收到链接消息且 title/linkUrl 非空
- **WHEN** QiWei 回调 payload 包含 msgType=13 且 msgData 中 title 和 linkUrl 非空
- **THEN** adapter 对 title/desc 做 Base64 解码，构造 messageType="link" 的 incomingMessage，content 格式为 `"[链接] 标题：{title}\n描述：{desc}\n地址：{linkUrl}"`

#### Scenario: 链接消息缺少 title
- **WHEN** msgType=13 但 title 为空
- **THEN** content 中 title 部分使用 linkUrl 作为替代

### Requirement: Adapter SHALL receive location messages as text

msgType=6 的位置消息 SHALL 被收敛为人类可读的文本。

#### Scenario: 收到完整位置消息
- **WHEN** QiWei 回调 payload 包含 msgType=6 且 msgData 中 address 和 latitude/longitude 非空
- **THEN** adapter 对 address/title 做 Base64 解码，构造 messageType="location" 的 incomingMessage，content 格式为 `"[位置] {title} {address} (纬度:{latitude}, 经度:{longitude})"`

#### Scenario: 位置消息缺少 title
- **WHEN** msgType=6 但 title 为空
- **THEN** content 中省略 title 部分，仅包含 address 和坐标

### Requirement: Adapter SHALL receive card (名片) messages as text

msgType=41 的名片消息 SHALL 被收敛为人类可读的文本。

#### Scenario: 收到名片消息
- **WHEN** QiWei 回调 payload 包含 msgType=41 且 msgData 中 nickname 非空
- **THEN** adapter 对 nickname/corpName 做 Base64 解码，构造 messageType="card" 的 incomingMessage，content 格式为 `"[名片] {nickname} 企业：{corpName}"`，channelMeta 中保存 shared_id

#### Scenario: 名片消息 corpName 为空
- **WHEN** msgType=41 且 corpName 为空
- **THEN** content 中省略企业部分，仅包含 nickname

### Requirement: Adapter SHALL receive red packet messages as notification text

msgType=26 的红包消息 SHALL 被收敛为只读文本通知。

#### Scenario: 收到红包消息
- **WHEN** QiWei 回调 payload 包含 msgType=26 且 msgData 中 wishingContent 非空
- **THEN** adapter 对 wishingContent 做 Base64 解码，构造 messageType="red_packet" 的 incomingMessage，content 格式为 `"[红包] {wishingContent}"`

#### Scenario: 红包消息 wishingContent 为空
- **WHEN** msgType=26 但 wishingContent 为空
- **THEN** content 为 `"[红包]"`

### Requirement: Adapter SHALL receive miniapp messages as text with preserved metadata

msgType=78 的小程序消息 SHALL 被收敛为人类可读文本，同时完整保留原始 msgData 到 channelMeta 以供后续转发。

#### Scenario: 收到小程序消息
- **WHEN** QiWei 回调 payload 包含 msgType=78 且 msgData 中 title 非空
- **THEN** adapter 对 title/desc 做 Base64 解码，构造 messageType="miniapp" 的 incomingMessage，content 格式为 `"[小程序] {title}\n{desc}"`，channelMeta 中保存完整的原始 msgData

#### Scenario: 小程序消息 desc 为空
- **WHEN** msgType=78 但 desc 为空
- **THEN** content 中省略 desc 行

### Requirement: Adapter SHALL receive channel_msg (视频号) messages as text

msgType=141 的视频号消息 SHALL 被收敛为人类可读文本。

#### Scenario: 收到视频号消息
- **WHEN** QiWei 回调 payload 包含 msgType=141 且 msgData 中 channelName 非空
- **THEN** adapter 对 channelName 做 Base64 解码，构造 messageType="channel_msg" 的 incomingMessage，content 格式为 `"[视频号] {channelName}\n链接：{channelUrl}"`

### Requirement: Adapter SHALL receive mixed (图文混合) messages

msgType=123 的图文混合消息 SHALL 被拆分处理：文字子消息拼接为 content，图片子消息走附件流程。

#### Scenario: 收到含文字和图片的混合消息
- **WHEN** QiWei 回调 payload 包含 msgType=123 且 msgData 为子消息数组，其中包含 subMsgType=2 (文本) 和 subMsgType=14 (图片)
- **THEN** adapter 构造 messageType="mixed" 的 incomingMessage，content 拼接所有文本子消息的 content（Base64 解码），图片子消息走现有 image 附件流程

#### Scenario: 混合消息只有文字
- **WHEN** msgType=123 且所有子消息都是 subMsgType=2
- **THEN** adapter 构造 messageType="text" 的 incomingMessage，content 为拼接后的文字

#### Scenario: 混合消息图片处理失败
- **WHEN** 混合消息中图片子消息的下载或 OSS 上传失败
- **THEN** 文字部分仍正常处理，图片部分降级为 `"[图片]"` 文本占位符
