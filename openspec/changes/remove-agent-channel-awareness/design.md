## Context

agent 当前以 `channel` 字段贯穿整条消息处理链路：channel-qiwei 适配器在入站 payload 中设置 `channel: "qiwei"`，dispatcher 校验白名单后写入 session 和 message，processor 在 `formatUserMessage` 中将其渲染为 `[via qiwei | ...]` 前缀注入 LLM 上下文，`send_channel_message` 工具把 `channel` 暴露为可选参数。

实际运行中只有企微一个通道。LLM 看到的 `via qiwei` 既无法帮助它做出更好的决策，也不应成为它需要关心的信息。

## Goals / Non-Goals

**Goals:**

1. LLM 可见的所有表面（用户消息元信息、工具 schema、工具返回值、system prompt）不再包含 channel 概念
2. wecom builtin tools 不再依赖 channel 门控，无条件注册
3. 内部数据流（dispatcher → runner → storage）保留 channel 字段用于日志和 session key，不做结构性删除

**Non-Goals:**

- 不重构 channel adapter registry（feishu/webui 注册逻辑保留，将来可能复用）
- 不修改数据库 schema 中的 channel 列
- 不修改 channel-qiwei 适配器自身代码
- 不修改 SQL seed 数据（053-wechat-builtin-agent.sql 等）

## Decisions

### D1: 从 formatUserMessage 中移除 `via {channel}` 元信息

`formatUserMessage` 当前签名接受 `channel` 参数并输出 `via qiwei`。方案是删除 channel 相关的 meta 拼接逻辑，不再在 `[...]` 前缀中包含渠道标识。

函数签名中的 `channel` 参数可以保留（调用方仍会传入），仅不再用于生成 LLM 可见文本。这样改动最小，避免大面积调用方签名变更。

### D2: 从 send_channel_message 工具定义中移除 channel 参数

从 Properties map 中删除 `"channel"` 键，LLM 不再看到这个参数。执行器内部仍从 `request.Channel` 取值（已有逻辑），不影响消息实际路由。

同时从返回值中移除 `"channel"` 字段。

### D3: 移除 registerWecomBuiltinTools 的 channel 门控

当前 `request.Channel != "qiwei"` 时直接 return，导致非 qiwei 请求不注册 wecom 工具。既然只有一个通道，这个判断多余。直接删除 if 块，让工具始终注册。

### D4: 更新 system prompt

`moli-system-prompt.md` 中两处引用 channel：
1. 消息格式说明 `[via 渠道 | msg_id=消息ID]` → 改为 `[msg_id=消息ID]`
2. send_channel_message 参数说明中的 `channel /` → 删除

### D5: 简化 dispatcher validChannels 校验

当前白名单 `{"feishu": true, "qiwei": true, "webui": true}` 校验入站消息的 channel 字段。保留校验逻辑但不是本次重点——channel-qiwei 传入的值仍为 `"qiwei"`，校验本身不影响 LLM。可以作为 P1 后续简化，本次不做强制变更。

**选择保留而非删除的原因**: dispatcher 的 channel 校验是防御性逻辑，防止未知来源注入，删除它的收益小于风险。

## Risks / Trade-offs

- **[风险] 历史 session 中已持久化的 `via qiwei` 文本** → 这些存在于 messages 表的 content 字段中。旧消息在回放时仍会带有 `via qiwei` 前缀。影响可忽略：LLM 看到旧格式不会报错，只是略显不一致。不做数据迁移。
- **[风险] webui 通道的 wecom 工具误注册** → 移除 channel 门控后，通过 webui 进入的请求也会注册 wecom 工具。但 webui 当前不活跃，且工具调用会因 channel-qiwei 适配器不可达而返回错误。可接受。
- **[取舍] 函数签名不做精简** → `formatUserMessage` 保留 channel 参数但不使用。比改签名触发大面积调用方修改更稳妥。
