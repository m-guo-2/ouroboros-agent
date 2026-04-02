# QiWe 开放平台集成文档

本文档记录项目对 QiWe 开放平台的集成现状，包括平台接口索引、回调事件处理逻辑、以及内部消息流转。

> 平台文档首页：`https://doc.qiweapi.com`

---

## 目录

- [1. 架构概览](#1-架构概览)
- [2. 平台 API 接口索引](#2-平台-api-接口索引)
- [3. 回调事件处理](#3-回调事件处理)
  - [3.1 回调入口](#31-回调入口)
  - [3.2 一级路由：cmd](#32-一级路由cmd)
  - [3.3 二级路由：msgType → 用户消息](#33-二级路由msgtype--用户消息)
  - [3.4 特殊消息：msgType 49 (appmsg) 按 subType 分流](#34-特殊消息msgtype-49-appmsg-按-subtype-分流)
  - [3.5 混合消息：msgType 123 按 subMsgType 拆解](#35-混合消息msgtype-123-按-submsgtype-拆解)
  - [3.6 群生命周期事件](#36-群生命周期事件)
  - [3.7 系统事件（cmd=15500）](#37-系统事件cmd15500)
  - [3.8 已知忽略的 msgType](#38-已知忽略的-msgtype)
- [4. 下游消息格式](#4-下游消息格式)
  - [4.1 用户消息 → agent](#41-用户消息--agent)
  - [4.2 群事件 → agent](#42-群事件--agent)
  - [4.3 agent → QiWe（出站消息）](#43-agent--qiwe出站消息)
- [5. channel-qiwei 服务路由表](#5-channel-qiwei-服务路由表)
- [6. 模块代理 API 映射](#6-模块代理-api-映射)

---

## 1. 架构概览

```
QiWe 平台
  │
  │  POST /webhook/callback (消息回调)
  ▼
channel-qiwei 服务
  ├─ 解析回调 → 按 cmd/msgType 路由
  ├─ 媒体管道：下载 → OSS 上传 → 语音 ASR 转写
  ├─ 构建 incomingMessage / GroupEvent
  │
  │  POST /api/channels/incoming     (用户消息)
  │  POST /api/channels/group-event  (群事件)
  ▼
agent 服务
  ├─ dispatcher → session → runner
  │
  │  POST /api/qiwei/send            (出站回复)
  │  POST /api/qiwei/{facade}        (工具调用)
  ▼
channel-qiwei 服务 → QiWe 平台 API
```

关键代码位置：
- 回调处理：`channel-qiwei/events.go`
- 数据模型：`channel-qiwei/models.go`
- 媒体管道：`channel-qiwei/media_pipeline.go`
- Facade 接口：`channel-qiwei/facade_handlers.go`
- 出站/代理：`channel-qiwei/api_handlers.go`、`channel-qiwei/cdn_upload.go`
- 模块注册：`channel-qiwei/internal/modules/`
- Agent 群事件：`agent/internal/dispatcher/group_event.go`
- Agent 工具注册：`agent/internal/runner/wecom_builtin_tools.go`

---

## 2. 平台 API 接口索引

### 开发指南

| 文档 | 链接 |
|------|------|
| 开发前必读 | https://doc.qiweapi.com/doc-7331301.md |
| 接入流程 | https://doc.qiweapi.com/doc-7562288.md |
| 消息订阅 | https://doc.qiweapi.com/doc-7331303.md |
| 消息回调内容说明 | https://doc.qiweapi.com/doc-7331304.md |
| 更新日志 | https://doc.qiweapi.com/doc-7331305.md |

### 实例管理

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 创建设备 | https://doc.qiweapi.com/api-344613850.md | `/client/createClient` |
| 恢复实例 | https://doc.qiweapi.com/api-344613851.md | `/client/restoreClient` |
| 停止实例 | https://doc.qiweapi.com/api-344613852.md | `/client/stopClient` |
| 设置回调地址 | https://doc.qiweapi.com/api-354411522.md | `/client/setCallback` |

### 登录模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 二维码-获取 | https://doc.qiweapi.com/api-344613856.md | `/login/getLoginQrcode` |
| 二维码-检测 | https://doc.qiweapi.com/api-344613857.md | `/login/checkLoginQrCode` |
| 二维码-code验证 | https://doc.qiweapi.com/api-344613858.md | `/login/verifyLoginQrcode` |
| 用户登录 | https://doc.qiweapi.com/api-344613859.md | `/login/manualLogin` |
| 用户状态 | https://doc.qiweapi.com/api-347221662.md | `/login/checkLogin` |

### 用户模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 生成二维码 | https://doc.qiweapi.com/api-344613861.md | `/user/getQrcodeCard` |
| 获取个人信息 | https://doc.qiweapi.com/api-344613862.md | `/user/getProfile` |
| 更新个人信息 | https://doc.qiweapi.com/api-344613863.md | `/user/setProfile` |
| 查询企业信息 | https://doc.qiweapi.com/api-344613864.md | `/user/getCorpInfo` |
| 注销 | https://doc.qiweapi.com/api-344613865.md | `/user/logout` |
| 个人收藏-分页 | https://doc.qiweapi.com/api-344613866.md | `/msg/syncCollectionMsg` |
| 个人收藏-添加GIF表情 | https://doc.qiweapi.com/api-344613867.md | `/msg/insertCollectionMsg` |

### 联系人模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 联系人详情-批量 | https://doc.qiweapi.com/api-344613868.md | `/contact/batchGetUserinfo` |
| 外部联系人分页 | https://doc.qiweapi.com/api-344613869.md | `/contact/getWxContactList` |
| 内部联系人分页 | https://doc.qiweapi.com/api-344613870.md | `/contact/getWxWorkContactList` |
| 联系人搜索 | https://doc.qiweapi.com/api-344613871.md | `/contact/searchContact` |
| 添加个微 | https://doc.qiweapi.com/api-344613872.md | `/contact/addSearchWxContact` |
| 添加企微 | https://doc.qiweapi.com/api-344613873.md | `/contact/addSearchWxWorkContact` |
| 添加群成员好友 | https://doc.qiweapi.com/api-425758709.md | — |
| 添加企微名片 | https://doc.qiweapi.com/api-344613874.md | `/contact/addCardContact` |
| 添加删除联系人 | https://doc.qiweapi.com/api-344613875.md | `/contact/addDeletedContact` |
| 同意申请 | https://doc.qiweapi.com/api-344613876.md | `/contact/agreeContact` |
| 个微联系人信息-更新 | https://doc.qiweapi.com/api-344613877.md | `/contact/updateWxContact` |
| 企微联系人信息-更新 | https://doc.qiweapi.com/api-344613878.md | `/contact/updateWxWorkContact` |
| 删除联系人 | https://doc.qiweapi.com/api-344613879.md | `/contact/deleteContact` |
| OpenID | https://doc.qiweapi.com/api-344613880.md | `/contact/openid` |

### 群模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 群分页 | https://doc.qiweapi.com/api-344613881.md | `/room/getRoomList` |
| 群详情-批量 | https://doc.qiweapi.com/api-344613882.md | `/room/batchGetRoomDetail` |
| 创建群 | https://doc.qiweapi.com/api-344613883.md | `/room/createRoom` |
| 修改群名称 | https://doc.qiweapi.com/api-344613884.md | `/room/modifyRoomName` |
| 修改群备注 | https://doc.qiweapi.com/api-344613885.md | `/room/modifyRoomRemarkName` |
| 修改群内昵称 | https://doc.qiweapi.com/api-344613886.md | `/room/modifyRoomNickname` |
| 邀请/添加成员 | https://doc.qiweapi.com/api-344613887.md | `/room/inviteRoomMember` |
| 移除成员 | https://doc.qiweapi.com/api-344613888.md | `/room/removeRoomMember` |
| 群二维码 | https://doc.qiweapi.com/api-344613889.md | `/room/getRoomQrCode` |
| 修改群公告 | https://doc.qiweapi.com/api-344613890.md | `/room/modifyRoomNotice` |
| 添加群管理员 | https://doc.qiweapi.com/api-344613891.md | `/room/roomAddAdmin` |
| 取消群管理员 | https://doc.qiweapi.com/api-344613892.md | `/room/roomRemoveAdmin` |
| 退群 | https://doc.qiweapi.com/api-344613893.md | `/room/quitRoom` |
| 转让群主 | https://doc.qiweapi.com/api-344613894.md | `/room/changeRoomMaster` |
| 群解散 | https://doc.qiweapi.com/api-344613895.md | `/room/dismissRoom` |
| OpenID | https://doc.qiweapi.com/api-344613896.md | `/room/openid` |
| 开启群改名 | https://doc.qiweapi.com/api-344613897.md | `/room/enableChangeRoomName` |
| 开启群邀请确认 | https://doc.qiweapi.com/api-344613898.md | `/room/openInviteConfirm` |
| 接受群邀请-By链接 | https://doc.qiweapi.com/api-410838558.md | `/room/agreeInviteByLink` |

### 消息模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 发送纯文本消息 | https://doc.qiweapi.com/api-344613906.md | `/msg/sendText` |
| 发送混合文本消息 | https://doc.qiweapi.com/api-344613907.md | `/msg/sendHyperText` |
| 发送图片消息 | https://doc.qiweapi.com/api-344613908.md | `/msg/sendImage` |
| 发送GIF表情消息 | https://doc.qiweapi.com/api-344613909.md | `/msg/sendGif` |
| 发送视频消息 | https://doc.qiweapi.com/api-344613910.md | `/msg/sendVideo` |
| 发送文件消息 | https://doc.qiweapi.com/api-344613911.md | `/msg/sendFile` |
| 发送语音消息 | https://doc.qiweapi.com/api-344613912.md | `/msg/sendVoice` |
| 发送链接消息 | https://doc.qiweapi.com/api-344613913.md | `/msg/sendLink` |
| 发送小程序消息 | https://doc.qiweapi.com/api-344613914.md | `/msg/sendWeapp` |
| 发送名片消息 | https://doc.qiweapi.com/api-344613915.md | `/msg/sendPersonalCard` |
| 发送视频号消息 | https://doc.qiweapi.com/api-344613916.md | `/msg/sendFeedVideo` |
| 发送定位消息 | https://doc.qiweapi.com/api-344613917.md | `/msg/sendLocation` |
| 撤回消息 | https://doc.qiweapi.com/api-344613918.md | `/msg/revokeMsg` |
| 修改消息状态 | https://doc.qiweapi.com/api-344613919.md | `/msg/statusModify` |
| 群消息置顶-列表 | https://doc.qiweapi.com/api-344613920.md | `/msg/roomTopMessageList` |
| 群消息置顶-添加 | https://doc.qiweapi.com/api-344613921.md | `/msg/roomTopMessageSet` |
| 群消息置顶-移除 | https://doc.qiweapi.com/api-344613922.md | `/msg/roomTopMessageSet` |
| 群发消息 | https://doc.qiweapi.com/api-344613923.md | `/msg/sendGroupMsg` |
| 群发消息-状态查询 | https://doc.qiweapi.com/api-344613924.md | `/msg/sendGroupMsgStatus` |
| 群发消息-规则查询 | https://doc.qiweapi.com/api-344613925.md | `/msg/sendGroupMsgRule` |
| 同步历史消息分页 | https://doc.qiweapi.com/api-344613926.md | `/msg/syncMsg` |

### 会话模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 会话分页 | https://doc.qiweapi.com/api-344613938.md | `/session/getSessionPage` |
| 会话组-编辑 | https://doc.qiweapi.com/api-344613939.md | `/session/setSessionCmd` |
| 会话组-查询 | https://doc.qiweapi.com/api-344613940.md | `/session/getSessionList` |

### 云存储 CDN 模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 文件上传-异步 | https://doc.qiweapi.com/api-395133885.md | `/cloud/cdnUploadByUrlAsync` |
| 文件上传 | https://doc.qiweapi.com/api-344613899.md | `/cloud/cdnBigUpload` |
| 文件上传-URL | https://doc.qiweapi.com/api-344613900.md | `/cloud/cdnBigUploadByUrl` |
| 企微文件下载 | https://doc.qiweapi.com/api-344613901.md | `/cloud/wxWorkDownload` |
| 企微文件下载（异步） | https://doc.qiweapi.com/api-389691087.md | `/cloud/wxWorkDownloadAsync` |
| 企微大文件下载（异步） | https://doc.qiweapi.com/api-389695362.md | `/cloud/cdnBigFileDownloadByUrlAsync` |
| 个微文件下载 | https://doc.qiweapi.com/api-344613902.md | `/cloud/wxDownload` |
| 文件CDN转URL | https://doc.qiweapi.com/api-344613903.md | `/cloud/cdnWxDownload` |
| 个微文件异步下载 | https://doc.qiweapi.com/api-399776006.md | `/cloud/wxDownloadAsync` |

### 朋友圈模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 列表分页 | https://doc.qiweapi.com/api-344613927.md | `/sns/getSnsRecord` |
| 获取详情-批量 | https://doc.qiweapi.com/api-344613928.md | `/sns/getSnsDetail` |
| 文件上传 | https://doc.qiweapi.com/api-344613929.md | `/sns/upload` |
| 发送朋友圈 | https://doc.qiweapi.com/api-344613930.md | `/sns/postSns` |
| 删除朋友圈 | https://doc.qiweapi.com/api-344613931.md | `/sns/deleteSns` |
| 点赞/取消赞 | https://doc.qiweapi.com/api-344613932.md | `/sns/snsLike` |
| 评论/追评 | https://doc.qiweapi.com/api-344613933.md | `/sns/snsComment` |
| 评论删除 | https://doc.qiweapi.com/api-344613934.md | `/sns/deleteSnsComment` |

### 标签模块

| 接口 | 平台文档 | 项目中使用的路径 |
|------|---------|---------------|
| 列表分页 | https://doc.qiweapi.com/api-361694421.md | `/label/syncLabelList` |
| 个人标签-增删改 | https://doc.qiweapi.com/api-344613936.md | `/label/editLabel` |
| 客户标签-增删 | https://doc.qiweapi.com/api-344613937.md | `/label/contactEditLabel` |

---

## 3. 回调事件处理

### 3.1 回调入口

- **Webhook 端点**：`POST /webhook/callback`
- **处理函数**：`handleWebhookCallback`（`channel-qiwei/events.go`）
- **响应**：始终立即返回 `{"code": 200, "msg": "ok"}`
- **异步处理**：每条消息在独立 goroutine 中处理，超时 20 秒

回调 body 支持多种格式：
- `{"data": [...]}`（标准格式）
- 裸数组 `[...]`
- 单个消息对象 `{...}`
- 验证回调（含 `验证回调地址是否可用` 字样）→ 直接丢弃

### 3.2 一级路由：cmd

`cmd` 字段缺省时默认为 `15000`。

| cmd | 含义 | 处理方式 | 代码位置 |
|-----|------|---------|---------|
| **15000** | 普通聊天/媒体消息 | `handleNormalMessage` → 按 msgType 分流 | `events.go:90` |
| **15500** | 系统事件 | `handleSystemEvent` → 群事件转发或仅记录日志 | `events.go:92` |
| **11016** | 账号状态变化 | 仅 `logger.Detail` 记录 | `events.go:94` |
| **20000** | API 异步消息 | 仅 `logger.Detail` 记录 | `events.go:97` |
| 其他 | 未知 cmd | 仅 `logger.Detail` 记录 | `events.go:100` |

### 3.3 二级路由：msgType → 用户消息

当 `cmd=15000` 时，`handleNormalMessage` 按 `msgType` 查表路由。映射表定义在 `events.go:19-46`（`userMessageTypeMap`）。

| msgType | 内部 messageType | 处理逻辑 | 发给 agent 的 content 格式 |
|---------|-----------------|---------|--------------------------|
| **0, 1, 2** | `text` | 提取 `msgData.content` 纯文本，空文本跳过 | 原始文本 |
| **3, 7, 14** | `image` | 媒体管道：QW 源下载 → OSS 上传 | `[收到图片]\n名称: xxx\n地址: oss://...` + attachment |
| **101** | `image` | 媒体管道：GW 源下载 → OSS 上传 | 同上 |
| **6** | `location` | `extractRichContent` 提取标题/地址/经纬度 | `[位置] 标题 地址 (纬度:xx, 经度:xx)` |
| **13** | `link` | `extractRichContent` 提取标题/描述/链接 | `[链接] 标题：xx\n描述：xx\n地址：xx` |
| **15, 20** | `file` | 媒体管道：QW 源下载 → OSS 上传 | `[收到文件]\n名称: xxx\n地址: oss://...` + attachment |
| **102** | `file` | 媒体管道：GW 源下载 → OSS 上传 | 同上 |
| **16, 34** | `voice` | 媒体管道：下载 → silk 转 wav → 火山引擎 ASR 转写 | `转写文本(语音消息)` 或 `[语音转写失败]` |
| **22, 23, 43** | `video` | 媒体管道：QW 源下载 → OSS 上传 | `[收到视频]\n名称: xxx\n地址: oss://...` + attachment |
| **103** | `video` | 媒体管道：GW 源下载 → OSS 上传 | 同上 |
| **26** | `red_packet` | `extractRichContent` 提取祝福语 | `[红包] 祝福语` 或 `[红包]` |
| **29** | `sticker` | 尝试媒体管道下载表情图片；失败则降级 | 图片 URL + attachment 或 `[表情]` |
| **104** | `sticker` | GW 源表情，同上 | 同上 |
| **41** | `card` | `extractRichContent` 提取昵称/企业名 | `[名片] 昵称 企业：企业名` |
| **49** | *(按 subType)* | `handleAppMessage` 二次路由，见 [3.4](#34-特殊消息msgtype-49-appmsg-按-subtype-分流) | 取决于 subType |
| **78** | `miniapp` | `extractRichContent` 提取标题/描述 | `[小程序] 标题\n描述` |
| **123** | `mixed` | `handleMixedMessage` 拆解子消息，见 [3.5](#35-混合消息msgtype-123-按-submsgtype-拆解) | 拼合文本 + 图片 attachments |
| **141** | `channel_msg` | `extractRichContent` 提取视频号名称/链接 | `[视频号] 名称\n链接：url` |

**媒体管道处理流程**（`prepareMediaForAgent`，`media_pipeline.go`）：

1. **分类** — 按 msgType 确定 `mediaSource`（qw/gw）和 `mediaKind`（image/file/voice/video）
2. **规范化** — 从 msgData 中提取 fileId、fileAesKey、cdnKey 等参数
3. **制定下载计划** — 优先级：CDN Key 直接下载 > fileId+AES 下载 > 平台 API 下载 > 直接 URL 下载
4. **执行下载** — 调用平台 CDN 接口或直接 HTTP GET
5. **后处理** — 语音走 ASR 转写；图片/视频/文件走 OSS 上传
6. **构建结果** — 返回 resourceURI（OSS 地址）+ attachment 元数据

### 3.4 特殊消息：msgType 49 (appmsg) 按 subType 分流

`handleAppMessage`（`events.go:424-457`）按 `subType` 字段路由。subType 的取值依次从 `msgData.subType`、`msgData.type`、`msgData.appmsgtype` 获取，若都为 0 则尝试从 `msgData.content` 解析 XML。

| subType | 内部 messageType | 处理逻辑 | 发给 agent 的内容 |
|---------|-----------------|---------|-----------------|
| **57** | `quote` | `contentFromQuote` 提取引用消息和回复文本 | 回复文本 + `channelMeta.quotedMessage`（含 msgSvrId、content、senderName） |
| **5** | `link` | `contentFromLink` 提取标题/描述/链接 | `[链接] 标题：xx\n描述：xx\n地址：xx` |
| **33, 36** | `miniapp` | `contentFromMiniapp` 提取标题/描述 | `[小程序] 标题\n描述` + `channelMeta.miniappData` |
| 其他 | `file` | 当 content 为空时走媒体管道上传文件 | 文件 URL + attachment |

### 3.5 混合消息：msgType 123 按 subMsgType 拆解

`handleMixedMessage`（`events.go:582-638`）将混合消息拆解为子消息数组，逐个处理：

| subMsgType | 处理方式 | 结果 |
|-----------|---------|------|
| **0, 2** | 提取 `subMsgData.content` 文本 | 拼入文本部分 |
| **7, 14, 101** | 走媒体管道下载图片 | 添加为 attachment |

文本部分用空格拼合；无文本时降级为 `[图文混合消息]`。

### 3.6 群生命周期事件

群事件可通过 `cmd=15000` 或 `cmd=15500` 两条路径到达，处理逻辑相同。映射表定义在 `events.go:107-113`（`groupEventTypes`）。

| msgType | 内部 eventType | 特殊处理 | 发给 agent 的 payload |
|---------|---------------|---------|---------------------|
| **1001** | `group_name_changed` | 清除群名缓存，调用 `/room/batchGetRoomDetail` 获取新群名 | `{"channel":"qiwei", "agentId":"...", "channelGroupId":"roomId", "eventType":"group_name_changed", "groupName":"新群名"}` |
| **1002** | `member_joined` | 无 | `{"channel":"qiwei", "agentId":"...", "channelGroupId":"roomId", "eventType":"member_joined"}` |
| **1003** | `member_removed` | 无 | `{"channel":"qiwei", ..., "eventType":"member_removed"}` |
| **1005** | `member_quit` | 无 | `{"channel":"qiwei", ..., "eventType":"member_quit"}` |
| **1023** | `group_dissolved` | agent 端将群状态标记为 `dissolved` | `{"channel":"qiwei", ..., "eventType":"group_dissolved"}` |

**agent 端处理**（`agent/internal/dispatcher/group_event.go`）：
- 验证 eventType 在允许列表中（额外允许 `group_created`，但平台目前不会产生该事件）
- 调用 `storage.UpsertChannelGroup` 写入/更新群记录
- `group_dissolved` 事件将群 status 设为 `dissolved`，其余为 `active`

**前置条件**：`fromRoomId` 不能为空或 `"0"`，否则跳过。

### 3.7 系统事件（cmd=15500）

`handleSystemEvent`（`events.go:641-662`）：

| msgType | 含义 | 处理方式 |
|---------|------|---------|
| 1001/1002/1003/1005/1023 | 群事件 | 委托给 `handleGroupEvent`，与 cmd=15000 相同 |
| **2357** | 好友申请 | `logger.Business` 记录 contactNickname、contactId，不转发 |
| **2132** | 好友申请(简) | `logger.Business` 记录，不转发 |
| 其他 | 未知系统事件 | `logger.Detail` 记录，不转发 |

### 3.8 已知忽略的 msgType

以下 msgType 在 `handleNormalMessage` 中被显式忽略（`knownIgnoredMsgTypes`，`events.go:116-120`）：

| msgType | 含义 | 处理 |
|---------|------|------|
| **146** | 直播 | 仅记录日志，不转发 |
| **2001** | 已读通知 | 仅记录日志，不转发 |
| **2005** | 未读通知 | 仅记录日志，不转发 |

不在 `userMessageTypeMap` 和 `groupEventTypes` 和 `knownIgnoredMsgTypes` 中的 msgType，记录 warn 日志后跳过。

---

## 4. 下游消息格式

### 4.1 用户消息 → agent

**端点**：`POST {AgentServer}/api/channels/incoming`

**body 结构**（`incomingMessage`，`models.go:36-50`）：

```json
{
  "channel": "qiwei",
  "channelUserId": "发送者 ID",
  "channelMessageId": "MsgSvrID（平台消息唯一标识）",
  "channelConversationId": "私聊=发送者 ID / 群聊=群 ID",
  "channelConversationName": "发送者昵称 / 群名称",
  "conversationType": "p2p | group",
  "messageType": "text | image | voice | video | file | link | location | card | red_packet | miniapp | channel_msg | sticker | mixed | quote",
  "content": "带发送者前缀的消息内容（格式：昵称[ID] 2026-03-31 12:00:00:消息内容）",
  "senderName": "发送者昵称",
  "timestamp": 1711843200000,
  "channelMeta": { "quotedMessage": {...}, "miniappData": {...}, "shared_id": "..." },
  "attachments": [
    {
      "id": "MsgSvrID:0",
      "kind": "image | file | video",
      "resourceUri": "oss://bucket/key",
      "displayName": "filename.jpg",
      "mimeType": "image/jpeg",
      "sourceMessageType": "image"
    }
  ],
  "agentId": "配置的 agent ID"
}
```

**content 前缀格式**：
- 有 ID：`昵称[senderId] 2026-03-31 12:00:00:消息内容`
- 无 ID：`昵称 2026-03-31 12:00:00:消息内容`
- 语音消息：`昵称[senderId] 2026-03-31 12:00:00:转写文本(语音消息)`

### 4.2 群事件 → agent

**端点**：`POST {AgentServer}/api/channels/group-event`

**body 结构**（`GroupEvent`，`group_event.go:12-18`）：

```json
{
  "channel": "qiwei",
  "agentId": "配置的 agent ID",
  "channelGroupId": "群 ID (roomId)",
  "eventType": "group_name_changed | member_joined | member_removed | member_quit | group_dissolved",
  "groupName": "仅 group_name_changed 时填充"
}
```

### 4.3 agent → QiWe（出站消息）

agent 通过 `POST {qiweiBaseURL}/api/qiwei/send` 发送消息，由 `handleSend`（`api_handlers.go`）处理。

**文本类消息** — 直接调用平台接口：

| messageType | 平台接口 | 参数 |
|-------------|---------|------|
| `text` | `/msg/sendText` | `{toId, content, reply?}` |
| `rich_text` | `/msg/sendHyperText` | `{toId, content, reply?}` |
| `link` | `/msg/sendLink` | `{toId, title, desc, linkUrl, iconUrl}` |
| `location` | `/msg/sendLocation` | `{toId, title, address, latitude, longitude}` |
| `miniapp` | `/msg/sendWeapp` | `{toId, ...channelMeta}` |

**媒体类消息** — 两步流程（CDN 上传 → 发送）：

1. 调用 `/cloud/cdnBigUploadByUrl` 上传资源到 CDN
2. 用 CDN 返回的参数调用对应发送接口

| messageType | CDN fileType | 发送接口 | 附加参数 |
|-------------|------------|---------|---------|
| `image` | 1 (jpg) | `/msg/sendImage` | fileKey, fileMd5, filename |
| `gif` | 1 (jpg) | `/msg/sendGif` | imgUrl (cloudUrl) |
| `file` | 5 (generic) | `/msg/sendFile` | filename |
| `voice` | 5 (generic) | `/msg/sendVoice` | voiceTime |
| `video` | 4 (mp4) | `/msg/sendVideo` | fileMd5, filename, coverImageSize, duration |

---

## 5. channel-qiwei 服务路由表

| 路由 | 方法 | 用途 | 调用方 |
|------|------|------|-------|
| `/health` | GET | 健康检查 | 运维 |
| `/api/health` | GET | 健康检查 | agent adapter |
| `/webhook/callback` | POST | 接收平台回调 | QiWe 平台 |
| `/api/qiwei/send` | POST | agent 出站消息 | agent channel adapter |
| `/api/qiwei/search_targets` | POST | 搜索联系人/群 | agent 内置工具 `wecom_search_targets` |
| `/api/qiwei/list_or_get_conversations` | POST | 会话列表/历史消息 | agent 内置工具 `wecom_list_or_get_conversations` |
| `/api/qiwei/parse_message` | POST | 解析消息内容 | agent 内置工具 `wecom_parse_message` |
| `/api/qiwei/send_message` | POST | Facade 发送消息 | agent 内置工具 `wecom_send_message` |
| `/api/qiwei/do` | POST | 通用 API 代理 | 管理端 |
| `/api/qiwei/{module}/{action}` | POST | 模块化 API 代理 | 管理端 |

---

## 6. 模块代理 API 映射

通过 `/api/qiwei/{module}/{action}` 可访问以下模块（`channel-qiwei/internal/modules/`）：

### instance

| action | 平台路径 |
|--------|---------|
| create-device | `/client/createClient` |
| resume | `/client/restoreClient` |
| stop | `/client/stopClient` |
| set-callback | `/client/setCallback` |

### login

| action | 平台路径 |
|--------|---------|
| get-qr | `/login/getLoginQrcode` |
| check-qr | `/login/checkLoginQrCode` |
| verify-code | `/login/verifyLoginQrcode` |
| user-login | `/login/manualLogin` |
| user-status | `/login/checkLogin` |

### user

| action | 平台路径 |
|--------|---------|
| create-qr | `/user/getQrcodeCard` |
| get-profile | `/user/getProfile` |
| update-profile | `/user/setProfile` |
| get-corp-info | `/user/getCorpInfo` |
| logout | `/user/logout` |
| list-favorites | `/msg/syncCollectionMsg` |
| add-favorite-gif | `/msg/insertCollectionMsg` |

### contact

| action | 平台路径 |
|--------|---------|
| batch-detail | `/contact/batchGetUserinfo` |
| list-external | `/contact/getWxContactList` |
| list-internal | `/contact/getWxWorkContactList` |
| search | `/contact/searchContact` |
| add-personal-wechat | `/contact/addSearchWxContact` |
| add-enterprise-wechat | `/contact/addSearchWxWorkContact` |
| add-wechat-card | `/contact/addCardContact` |
| re-add | `/contact/addDeletedContact` |
| approve-request | `/contact/agreeContact` |
| update-personal | `/contact/updateWxContact` |
| update-enterprise | `/contact/updateWxWorkContact` |
| delete | `/contact/deleteContact` |
| get-openid | `/contact/openid` |

### group

| action | 平台路径 |
|--------|---------|
| list | `/room/getRoomList` |
| batch-detail | `/room/batchGetRoomDetail` |
| create | `/room/createRoom` |
| rename | `/room/modifyRoomName` |
| remark | `/room/modifyRoomRemarkName` |
| set-nickname | `/room/modifyRoomNickname` |
| add-member | `/room/inviteRoomMember` |
| remove-member | `/room/removeRoomMember` |
| qrcode | `/room/getRoomQrCode` |
| set-notice | `/room/modifyRoomNotice` |
| add-admin | `/room/roomAddAdmin` |
| remove-admin | `/room/roomRemoveAdmin` |
| quit | `/room/quitRoom` |
| transfer-owner | `/room/changeRoomMaster` |
| dismiss | `/room/dismissRoom` |
| get-openid | `/room/openid` |
| enable-rename | `/room/enableChangeRoomName` |
| enable-invite-confirm | `/room/openInviteConfirm` |
| accept-invite-by-link | `/room/agreeInviteByLink` |

### message

| action | 平台路径 |
|--------|---------|
| send-text | `/msg/sendText` |
| send-hyper-text | `/msg/sendHyperText` |
| send-image | `/msg/sendImage` |
| send-gif | `/msg/sendGif` |
| send-video | `/msg/sendVideo` |
| send-file | `/msg/sendFile` |
| send-voice | `/msg/sendVoice` |
| send-link | `/msg/sendLink` |
| send-mini-program | `/msg/sendWeapp` |
| send-card | `/msg/sendPersonalCard` |
| send-channel-video | `/msg/sendFeedVideo` |
| send-location | `/msg/sendLocation` |
| revoke | `/msg/revokeMsg` |
| update-status | `/msg/statusModify` |
| list-top | `/msg/roomTopMessageList` |
| add-top | `/msg/roomTopMessageSet` |
| remove-top | `/msg/roomTopMessageSet` |
| mass-send | `/msg/sendGroupMsg` |
| mass-send-status | `/msg/sendGroupMsgStatus` |
| mass-send-rule | `/msg/sendGroupMsgRule` |
| sync-history | `/msg/syncMsg` |

### cdn

| action | 平台路径 |
|--------|---------|
| upload-async | `/cloud/cdnUploadByUrlAsync` |
| upload | `/cloud/cdnBigUpload` |
| upload-url | `/cloud/cdnBigUploadByUrl` |
| download-qw-file | `/cloud/wxWorkDownload` |
| download-qw-file-async | `/cloud/wxWorkDownloadAsync` |
| download-qw-large-async | `/cloud/cdnBigFileDownloadByUrlAsync` |
| download-gw-file | `/cloud/wxDownload` |
| cdn-to-url | `/cloud/cdnWxDownload` |
| download-gw-async | `/cloud/wxDownloadAsync` |

### moment

| action | 平台路径 |
|--------|---------|
| list | `/sns/getSnsRecord` |
| batch-detail | `/sns/getSnsDetail` |
| upload-media | `/sns/upload` |
| publish | `/sns/postSns` |
| delete | `/sns/deleteSns` |
| like | `/sns/snsLike` |
| comment | `/sns/snsComment` |
| delete-comment | `/sns/deleteSnsComment` |

### tag

| action | 平台路径 |
|--------|---------|
| list | `/label/syncLabelList` |
| edit-personal-tag | `/label/editLabel` |
| edit-customer-tag | `/label/contactEditLabel` |

### session

| action | 平台路径 |
|--------|---------|
| list | `/session/getSessionPage` |
| edit-group | `/session/setSessionCmd` |
| get-group | `/session/getSessionList` |
