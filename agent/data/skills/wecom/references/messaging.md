## 消息管理

通过 wecom_api 工具调用以下方法。

### 撤回消息
- method: `/msg/revokeMsg`
- params: `{msgSvrId: "消息ID", toId: "接收者ID"}`

### 置顶消息
- method: `/msg/roomTopMessageSet`
- params: `{roomId: "群ID", msgSvrId: "消息ID", action: 1}`
- action: 1=置顶, 0=取消置顶

### 列出置顶消息
- method: `/msg/roomTopMessageList`
- params: `{roomId: "群ID"}`

### 群发消息
- method: `/msg/sendGroupMsg`
- params: `{toIds: ["id1","id2"], content: "消息内容", msgType: 1}`

### 查询群发状态
- method: `/msg/sendGroupMsgStatus`
- params: `{msgId: "群发ID"}`

### 同步历史消息
- method: `/msg/syncMsg`
- params: `{toId: "会话ID", msgSvrId: "起始消息ID"}`

### 发送富文本消息
- method: `/msg/sendHyperText`
- params: `{toId: "接收者ID", content: "消息XML"}`

### 发送链接消息
- method: `/msg/sendLink`
- params: `{toId: "接收者ID", title: "标题", desc: "描述", linkUrl: "URL", imgUrl: "缩略图URL"}`

### 发送位置
- method: `/msg/sendLocation`
- params: `{toId: "接收者ID", longitude: "经度", latitude: "纬度", label: "地名"}`

### 发送名片
- method: `/msg/sendPersonalCard`
- params: `{toId: "接收者ID", userId: "名片用户ID"}`