## 会话管理

通过 wecom_api 工具调用以下方法。

### 获取会话列表（分页）
- method: `/session/getSessionPage`
- params: `{pageNum: 1, pageSize: 20}`

### 获取会话分组
- method: `/session/getSessionList`
- params: `{}`

### 编辑会话分组
- method: `/session/setSessionCmd`
- params: `{sessionId: "会话ID", cmd: "操作类型"}`