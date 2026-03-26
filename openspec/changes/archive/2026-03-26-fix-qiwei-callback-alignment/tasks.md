## 1. 补全缺失的 msgType 映射

- [x] 1.1 在 `userMessageTypeMap` 中添加 `20 → "file"` 和 `22 → "video"`
- [x] 1.2 在 `mediaClassifications` 中添加 msgType 20（source=qw, kind=file）和 msgType 22（source=qw, kind=video）
- [x] 1.3 在 `handleNormalMessage` 中对已知非 agent 相关 msgType（146, 2001, 2005）记录 Detail 日志而非 Warn

## 2. fileNameExt 字段利用

- [x] 2.1 新增 `extensionFromFileNameExt(value string) string` 辅助函数，实现 fileNameExt 到扩展名的映射
- [x] 2.2 在 `normalizeMediaDescriptor` 中读取 `msgData["fileNameExt"]`，当文件名缺少扩展名时拼接

## 3. agent-server 群数据存储

- [x] 3.1 在 `storage/db.go` 的 migrations 中添加 `channel_groups` 表 DDL 和索引
- [x] 3.2 在 `storage/` 中新增 `groups.go`，实现 `UpsertChannelGroup` 和 `GetChannelGroup` 函数
- [x] 3.3 在 `dispatcher/` 中新增 `HandleGroupEvent` HTTP handler
- [x] 3.4 在 `main.go` 中注册 `/api/channels/group-event` 路由

## 4. channel-qiwei 群事件上报

- [x] 4.1 实现 `reportGroupEvent(ctx, eventType, msg)` 方法，POST 到 agent-server 的 `/api/channels/group-event`
- [x] 4.2 在 `handleNormalMessage` 路由中，对群事件 msgType（1001/1002/1003/1005/1023）提前拦截，调用 `reportGroupEvent`
- [x] 4.3 在 `handleSystemEvent` 的 1002 处理中也增加 `reportGroupEvent` 调用
- [x] 4.4 从 `handleGroupMemberJoined` 中移除向 agent 转发系统消息的逻辑（群事件不再推给 agent）

## 5. 测试与验证

- [x] 5.1 为 msgType 20/22 添加 `normalizeMediaDescriptor` 单元测试
- [x] 5.2 为 `extensionFromFileNameExt` 添加单元测试
- [x] 5.3 为群事件路由添加 `handleNormalMessage` 单元测试
- [x] 5.4 运行 `go vet` 和 `go build` 确认 channel-qiwei 和 agent 均编译通过
