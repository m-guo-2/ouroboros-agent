## Why

agent 内部对 channel（渠道）的抽象——feishu / qiwei / webui 三通道路由——在当前阶段造成了不必要的认知负担。实际只有企微（qiwei）一个活跃通道，但 LLM 每条消息都看到 `[via qiwei | msg_id=...]` 前缀，`send_channel_message` 工具还暴露 `channel` 参数让模型觉得需要选择渠道。这些多余信息浪费 token、分散注意力，且没有实际用途。

## What Changes

- 移除 `formatUserMessage` 中 `via {channel}` 元信息标签，LLM 不再看到渠道标识
- 移除 `send_channel_message` 工具定义中的 `channel` 参数，内部硬编码当前请求的 channel
- 移除 `send_channel_message` 返回值中的 `channel` 字段
- 更新 `moli-system-prompt.md`，删除 `via 渠道` 格式说明和 `channel` 参数说明
- 移除 `registerWecomBuiltinTools` 中 `request.Channel != "qiwei"` 门控，wecom 工具无条件注册
- 简化 `dispatcher.go` 中 `validChannels` 白名单校验

## Capabilities

### New Capabilities

- `strip-channel-from-prompt`: 从 LLM 可见的用户消息元信息和工具 schema 中移除 channel 概念，降低 agent 认知负担

### Modified Capabilities

（无现有 spec 需要修改）

## Impact

- **agent/internal/runner/processor.go**: `formatUserMessage` 签名变更、`send_channel_message` 工具定义和执行器变更
- **agent/internal/runner/wecom_builtin_tools.go**: 移除 channel 门控
- **agent/internal/dispatcher/dispatcher.go**: 简化入口校验
- **agent/data/moli-system-prompt.md**: prompt 文本变更
- **不影响**: channel-qiwei 适配器自身代码、数据库 schema、storage 层 channel 字段（这些在内部数据流中保留，只是不再向 LLM 暴露）
