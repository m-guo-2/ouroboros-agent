# 企微四接口沟通门面

- **日期**：2026-03-09
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

`channel-qiwei` 之前同时承担了简单发送入口、raw API 透传和回调接入，但对 agent 来说仍然暴露了过多企微原始概念。模型需要记住联系人搜索、群列表、历史消息同步、通用 API 等不同入口，认知负担偏高。

同时，图片、文件、语音等非文本消息还缺少一条统一的解析链路，难以支撑 agent 与人进行更自然的多模态沟通。

## 决策

将 `channel-qiwei` 固定为工程适配层，对 agent 直接暴露 4 个语义化接口：

- `search_targets`
- `list_or_get_conversations`
- `parse_message`
- `send_message`

其中多模态识别通过独立 provider 接口接入，第一版预留火山能力接入点，但不把火山 API 变成系统中心。

## 变更内容

- 在 `channel-qiwei` 增加 4 个 façade 路由，统一封装联系人/群搜索、会话读取、消息解析和发送。
- 为消息解析新增标准化结构、附件抽取、下载与识别 provider 骨架。
- 在 `channel-qiwei` 配置中加入火山图片理解、文档理解、语音识别所需的环境变量入口。
- 将 `agent/data/043-wecom-skills-apply.py` 与 `agent/data/043-wecom-skills.sql` 的 `wecom-core` 工具定义切换到新的 4 个 façade 接口。

## 考虑过的替代方案

### 继续对 agent 暴露 raw API / 通用 API

没有采用。虽然实现最省事，但模型需要理解过多企微底层 method、参数和数据形状，工具调用稳定性差。

### 在 agent 内部再包一层 communication layer

没有采用。这样会让 `channel-qiwei` 和 agent 之间再次出现边界泄漏，企微细节仍然会向上冒泡。更合适的做法是在 `channel-qiwei` 内部直接完成 façade 收口。

## 影响

- agent 侧对企微能力的调用面显著收缩到 4 个稳定工具。
- `channel-qiwei` 成为明确的“工程适配层 + 语义门面层”，而不是单纯 raw passthrough。
- 当前阶段不做缓存、降级和静默兜底，优先暴露多模态解析链路中的真实问题。
- `parse_message` 已具备图片与语音识别 provider 骨架，文档理解仍保留显式未完成状态，后续需要继续补齐。
