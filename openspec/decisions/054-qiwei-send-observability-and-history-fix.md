# 企微发送可观测性与历史消息修复

- **日期**：2026-03-09
- **类型**：Bug 修复
- **状态**：已实施

## 背景

企微 `wecom_send_message` 出现了“工具返回成功，但用户端没有明显可见结果”的排查困难。现有 `channel-qiwei` 发送链路只把下游结果透传给 agent，没有记录关键请求与响应，导致无法区分是发送未落到下游、下游已接收但未展示，还是历史查询层把证据吞掉了。

同时，会话历史 façade 在读取 `/msg/syncMsg` 时没有兼容真实返回字段 `syncMsgList`，导致历史消息接口经常返回空列表，进一步放大了误判。

## 决策

保留现有四接口 façade 设计，但补齐发送链路日志和历史消息兼容层，让发送问题先变得可观察，再继续定位下游平台行为。

## 变更内容

- 在 `channel-qiwei/facade_handlers.go` 中将历史消息提取从 `messageList/msgList/...` 扩展到 `syncMsgList`。
- 在 `channel-qiwei/facade_handlers.go` 中修正历史消息归一化，兼容 `msgServerId` 和 `msgData.content`。
- 在 `channel-qiwei/facade_handlers.go` 中为 `send_message` 增加发送开始、失败、成功三段日志。
- 在 `channel-qiwei/qiwei_client.go` 中为下游 `/api/qw/doApi` 请求增加请求日志、失败日志、响应日志，保留截断后的 body 便于终端直接排查。
- 在 `channel-qiwei/events.go` 中将回调消息被跳过的关键分支（重复消息、不支持的 `msgType`、空文本）提升到可见日志级别，方便排查图片/文件等消息为何未进入 agent。
- 在 `channel-qiwei/events.go` 中为回调链路补充附件预下载：图片、文件、语音在转发给 agent 前先落本地，并把 `localPath` 摘要写入消息内容与 `ChannelMeta.attachments`。
- 在 `channel-qiwei/events.go` / `channel-qiwei/facade_handlers.go` 中补充 `msgType=101` 图片、`msgType=16` 语音兼容，以及 `fileBigHttpUrl` / `fileHttpUrl` 等下载字段识别。
- 在 `channel-qiwei/facade_handlers.go` 中为附件下载增加官方接口兜底：直链下载失败后，自动尝试 `/cloud/cdnWxDownload` 或 `/cloud/wxWorkDownload`，使用 `fileId`、`fileAeskey`、`fileMd5`、`fileSize`、`fileType` 重新换取可下载地址。
- 在 `channel-qiwei/server.go` 中让 `app` 与 `qiweiClient` 共享同一个 logger，避免日志割裂。

## 考虑过的替代方案

### 只修历史消息解析

没有采用。这样只能改善“查不到证据”的问题，仍然看不到真实发送请求和下游响应，下一次类似故障还是需要盲猜。

### 把下游完整响应结构上抛给 agent

没有采用。诊断信息首先应留在渠道适配层日志里，否则会让 agent 再次理解过多企微底层细节，违背 façade 收口的目标。

## 影响

- 后续复现发送异常时，可以直接在 `channel-qiwei` 日志中看到 method、toId、请求参数预览和下游响应体。
- `list_or_get_conversations` 现在能正确读到 `/msg/syncMsg` 的主要结果，便于人工排查和 agent 读取上下文。
- 如果问题仍然存在，下一步就可以基于日志继续判断是企微客户端展示问题，还是下游平台异步发送语义导致的延迟/丢弃。
