## Context

`channel-qiwei` 是 agent 与企业微信之间的桥接层，通过 HTTP 回调接收消息。当前实现基于 qiweapi bridge 的回调数据结构，核心路由逻辑为：

1. `handleCallbackMessage` 按 `cmd` 分发：`15000` → `handleNormalMessage`，`15500` → `handleSystemEvent`
2. `handleNormalMessage` 按 `userMessageTypeMap[msgType]` 查找消息类型，再按类型分支处理
3. `handleSystemEvent` 按 `msgType` 处理系统事件（当前仅处理 1002 入群）

与官方文档对照后发现多处不一致（详见 proposal.md）。核心矛盾在于：

- **群生命周期事件**（1001/1002/1003/1005/1023）在文档中以 `cmd=15000` 到达，但不在 `userMessageTypeMap` 中，被 `handleNormalMessage` 静默丢弃
- **msgType 20（文件）和 22（视频）**文档明确列出但代码遗漏
- **`fileNameExt` 字段**文档示例中存在但代码从未读取

agent-server 侧当前没有独立的群信息表，群上下文仅通过 `agent_sessions` 的 `channel_conversation_id` 和 `channel_name` 间接体现。

## Goals / Non-Goals

**Goals:**
- 补全 `userMessageTypeMap` 中缺失的 msgType 20 和 22
- 在 agent-server 侧新建 `channel_groups` 表，存储群基础信息（名称、状态）
- 新增 `/api/channels/group-event` API，接收渠道上报的群事件并更新 `channel_groups` 表
- `channel-qiwei` 收到群事件后翻译为统一格式，POST 到 agent-server 的 group-event API
- 兼容 bridge 可能以 cmd=15000 或 cmd=15500 发送群事件的两种情况
- 利用 `fileNameExt` 改善企微文件的文件名推断

**Non-Goals:**
- 不存储群成员列表（成员查询走 bridge API 实时拉取，后续按需扩展）
- 不处理联系人变动类系统事件（2131, 2188 等）
- 不处理标签变动（2160, 2161, 2185, 2186）
- 不处理 msgType 146（直播）、2001/2005（已读/未读通知）— 仅静默日志
- 不在本次实现 Persona 分配（属于 `per-group-agent-profile` 变更）
- 不向 agent 推送群事件消息 — 群事件只做数据更新，agent 需要时自行查询

## Decisions

### Decision 1: agent-server 作为群数据的统一存储层

**选择**: 在 agent-server 的 SQLite 中新建 `channel_groups` 表，channel 适配器通过 HTTP API 上报群事件。

**替代方案**: 在每个 channel 适配器中自建存储 → 导致多份数据、agent-server 查询群信息需跨服务调用。

**理由**: agent-server 是唯一有持久存储的中心节点，且 `per-group-agent-profile` 的 Persona 分配也在此层。channel 适配器保持无状态协议翻译器角色，新增渠道只需实现同一个 group-event 上报协议。

### Decision 2: channel_groups 表设计

```sql
CREATE TABLE IF NOT EXISTS channel_groups (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    channel_group_id TEXT NOT NULL,
    group_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_groups_agent_channel_group
    ON channel_groups(agent_id, channel, channel_group_id);
```

- `channel_group_id`: 渠道侧的群 ID（qiwei 的 roomId, feishu 的 chatId）
- `status`: `active` / `dissolved`
- 不存储成员列表（后续按需扩展列或建关联表）

### Decision 3: group-event API 设计

```
POST /api/channels/group-event
{
    "channel": "qiwei",
    "agentId": "xxx",
    "channelGroupId": "239655862281126",
    "eventType": "group_name_changed" | "member_joined" | "member_removed" | "member_quit" | "group_dissolved",
    "payload": { ... }
}
```

handler 逻辑：按 eventType UPSERT `channel_groups` 表。`group_dissolved` 将 status 设为 `dissolved`。`group_name_changed` 更新 `group_name`。其余事件仅更新 `updated_at`（成员数据待后续扩展）。

### Decision 4: channel-qiwei 群事件路由

在 `handleNormalMessage` 的路由中，对 msgType 属于群事件集合（1001/1002/1003/1005/1023）的消息，提前拦截，调 `reportGroupEvent` 上报给 agent-server，不走普通消息处理流程。

保留 `handleSystemEvent` 中 1002 的现有处理（双路径兼容），同时在该路径中也增加 group-event 上报。

### Decision 5: fileNameExt 作为文件名扩展名推断的辅助来源

在 `normalizeMediaDescriptor` 中，当 `fileName` 解码后缺少扩展名时，用 `fileNameExt` 字段推断扩展名（excel→.xlsx, word→.docx, ppt→.pptx, pdf→.pdf）并拼接。

### Decision 6: msgType 20/22 直接补入映射表

`20 → "file"` + `mediaKindFile`，`22 → "video"` + `mediaKindVideo`。与 15/23 共用现有媒体下载管线。

## Risks / Trade-offs

- **[Risk] Bridge 实际行为与文档不一致** → 群事件同时在 cmd=15000 和 cmd=15500 两条路径处理。msgType 20/22 如 bridge 不实际发送，映射也不会出错。
- **[Risk] agent-server 不可达时群事件丢失** → channel-qiwei 上报失败仅记录 Warn 日志，不阻塞。下次收到群消息时会重新触发 session 创建，群名等信息通过现有的 session channel_name 字段也能部分补偿。
- **[Trade-off] 不存储成员列表** → 简化本次实现。成员信息通过 bridge API 实时查询。后续可扩展 `channel_group_members` 表。
- **[Trade-off] 保留冗余 1002 双路径** → 增加少量代码冗余，换来对 bridge 行为不确定性的兼容性。
