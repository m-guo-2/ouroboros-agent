# QiWe Media Pipeline Design

## Context

当前 `channel-qiwei` 的媒体管线已经把 `msgType` 语义、企微/个微下载契约、OSS 物化和语音转写收到了明确阶段里，解决了“下载走错接口”和“入口细节泄漏过多”的第一层问题。但新的真实运行现象暴露出第二层问题：

1. 图片、文件、视频进入 agent 时仍被压平成一段正文字符串，资源地址只是文本里的一部分。
2. agent 想继续理解附件内容时，需要自己从正文里识别链接，再主动拼出一次 `wecom_parse_message(resourceUri=...)` 调用。
3. 这个协议过弱，导致模型即使拿到了可解析资源，也可能直接回复“我没有视觉能力”，而不是稳定触发解析能力。

这次设计不回到“富媒体统一前置解析”的路线，而是在现有媒体管线之上继续收口出一个更稳定的职责边界：

- `voice` 仍然是入口层特例，必须前置转写。
- `image/file/video` 只前置做资源准备，不前置做语义理解。
- agent 接收结构化附件，而不是正文中的资源地址提示。
- 当用户问题确实依赖附件内容时，runtime 约束 agent 必须显式调用统一附件分析能力。

## Goals / Non-Goals

**Goals:**

- 保留并复用现有媒体管线的分类、归一化、下载规划、来源隔离和 OSS 物化能力。
- 让语音继续在 `channel-qiwei` 内前置转写，agent 只消费文本。
- 让图片、文件、视频以结构化附件形式进入 agent，而不是只作为正文中的资源地址提示存在。
- 引入统一的按需附件分析能力，供 agent 在真正需要时显式调用 OCR、图片理解、文档抽取和后续可扩展的视频理解。
- 在 agent/runtime 侧加入 attachment-aware 约束，避免“该分析附件时却直接文字搪塞”的情况。
- 保持 `qw/gw` 来源、下载策略、规划失败和识别错误等内部诊断信息仍然只存在于入口层和日志里。

**Non-Goals:**

- 不把图片、文件、视频改成默认前置 OCR/视觉/文档摘要。
- 不改变 `search_targets`、`list_or_get_conversations`、`send_message` 这些 façade 的对外业务语义。
- 不要求 agent 理解企微/个微下载契约、对象存储细节或 provider 选择逻辑。
- 不在本次设计里讨论预签名 URL、公开链接权限模型等 OSS 更上层能力。
- 不在第一阶段一次性支持所有附件任务类型，只先覆盖当前最需要的 `describe/ocr/extract/transcribe` 语义。

## Decisions

### 1. 保留当前媒体管线前半段，但重新定义 agent-facing 输出

统一媒体管线仍然维持：

`raw message -> classifier -> normalizer -> planner -> resolver -> materializer`

但 agent-facing 输出不再是单一 `content string`，而是：

- `text`: 用户可直接消费的文本主体
- `attachments[]`: 已准备好的附件对象列表

理由：

- 下载契约、来源判断、OSS 物化这些仍然是入口层职责，不应该回退。
- 真正不稳定的点不是下载，而是“附件进入 agent 之后被当成正文附注”。

### 2. 语音继续作为入口层特例，前置转写后再进入 agent

语音消息仍然走专用通道：

- `channel-qiwei` 下载语音资源
- 内部调用 ASR 完成转写
- agent 只接收转写文本
- 若失败，只看到统一降级文本，不看到语音附件状态

理由：

- 语音原始资源对 agent 几乎不可直接消费，转成文本后才能进入正常语言推理。
- 语音的高价值结果是文本，而不是文件本身；与图片/文件“是否需要继续看内容”不是同类问题。

### 3. 图片、文件、视频只前置准备资源，不前置做内容理解

对于 `image/file/video`：

- 入口层负责分类、归一化、下载、OSS 物化
- 不默认执行 OCR、视觉理解、文档摘要或视频摘要
- 入口层只向 agent 暴露结构化附件最小字段

最小附件对象建议包含：

- `attachmentId`
- `kind`
- `resourceUri`
- `displayName?`
- `mimeType?`
- `sourceMessageType`

不包含：

- `qw/gw` 来源
- 下载策略
- 契约字段
- 失败阶段
- provider 原始错误

理由：

- 用户并不是每次收到图片/文件都要让 agent 理解内容。
- 提前做视觉/OCR/文档解析既增加时延，也会浪费 provider 成本。
- 把“可分析附件”建成一等对象，比让模型从正文里抠链接可靠得多。

### 4. 新增统一的按需附件分析能力，替代正文协议

新增平台级 builtin/tool 能力，例如 `inspect_attachment`，而不是继续把主流程建立在 `wecom_parse_message(resourceUri=...)` 上。

建议输入：

- `attachmentId`
- `task`
- `options?`

建议任务类型：

- `describe_image`
- `ocr_image`
- `extract_text`
- `summarize_document`
- `summarize_video`

返回：

- `status`
- `task`
- `text`
- `summary?`
- `structuredData?`

其中：

- `wecom_parse_message` 可保留为兼容 façade
- 但主流程由 `inspect_attachment` 承担
- `inspect_attachment` 内部可复用 `channel-qiwei` 现有解析逻辑，或逐步上移成共享附件分析层

理由：

- 当前最大的不稳定性来自“工具协议是字符串级隐式约定”。
- agent 应该操作 attachment 对象，而不是自己从正文里拼装资源地址。

### 5. 在 runtime 增加 attachment-aware 约束，而不是完全依赖模型自觉

系统不做附件内容前置解析，但也不能把 lazy parsing 的触发权完全交给 LLM 自由发挥。

因此在 runner/runtime 层增加一层约束：

- 若当前用户输入包含未解析附件，且用户问题明显依赖附件内容，agent 必须先调用 `inspect_attachment`
- 若 assistant 试图直接输出“看不到/无法识别/请你自己描述图片”等逃避式回复，runtime 可视为无效回答并要求重新决策

一个简单判断标准是：

- 用户显式问“这张图是什么”“帮我读一下文件”“视频里讲了什么”
- 当前 turn 中附件是主要信息来源，正文不足以回答

理由：

- “按需调用”不等于“完全靠模型自觉”。
- 需要一个工程化约束来保证该调用时一定调用，而不是继续回到 prompt 运气学。

### 6. `callback` 与 `parse_message` 共享资源准备，但不再共享同一种 agent 协议

两条入口仍共享：

- classifier
- normalizer
- planner
- resolver
- materializer

但上游消费协议区分为两类：

- `callback -> agent incoming`: 文本 + 结构化附件
- `parse_message` / `inspect_attachment`: 针对单个附件执行按需理解

其中：

- `parse_message` 可以继续兼容旧入参 `resourceUri` / `localPath`
- 新主路径优先基于 `attachmentId`

理由：

- 资源准备逻辑本来就应该共享。
- “转发给 agent 的消息”与“按需分析单个附件”不是同一种抽象，不应再强绑在一起。

### 7. 诊断信息继续留在入口内部，但分析失败状态需要结构化返回给 agent

下载、来源、契约、执行阶段等诊断信息继续只存在于 `channel-qiwei` 内部日志。

但按需附件分析工具本身需要返回结构化失败语义，例如：

- `provider_unconfigured`
- `download_failed`
- `unsupported_format`
- `analysis_timeout`

返回给 agent 的是稳定失败码和简洁说明，不是 provider 原始报错。

理由：

- 否则模型又会编造“我没有视觉能力”这种不准确边界。
- structured failure 让 agent 能正确向用户解释“这次没看成”而不是误判系统能力。

## Risks / Trade-offs

- [链路跨模块扩展] → 这次不再是 `channel-qiwei` 单模块改造，必须同步扩展 agent 入站协议和 builtin/tool 边界。
- [消息 schema 变更影响现有存储与历史格式化] → 需要以向后兼容方式扩展 message 结构，避免破坏纯文本渠道与历史消息。
- [attachment-aware 约束过严] → 只在“回答显著依赖附件内容”时触发，不把普通闲聊、确认、转发场景都强制分析。
- [tool 职责迁移带来重复实现] → 初期允许 `inspect_attachment` 复用 `wecom_parse_message` 内部逻辑，先收口契约，再决定是否抽共享分析层。
- [视频理解范围过大] → 第一阶段只定义扩展位和统一接口，不强行承诺高质量视频语义理解。

## Migration Plan

1. 保留现有媒体分类、下载、OSS 物化和语音转写实现不变。
2. 先扩展 `channel-qiwei -> agent` 入站 schema，引入结构化 `attachments[]`，同时兼容旧 `content string`。
3. 在 agent runtime 中补充 attachment-aware 消息建模与历史重建逻辑。
4. 新增 `inspect_attachment` builtin/tool，并让其先复用现有 `parse_message` / recognizer 能力。
5. 将 prompt 与 tool 协议从“看到资源地址就自己调用”迁移到“看到附件对象时按任务调用”。
6. 为 runtime 增加“需要附件内容时不能跳过分析”的校验逻辑。
7. 待主路径稳定后，将 `wecom_parse_message` 降级为兼容入口或内部桥接层。

## Open Questions

- 结构化附件是作为 `incomingMessage.ChannelMeta.attachments` 先向后兼容落地，还是直接升级为一等顶层字段。
- `attachmentId` 的稳定性边界是什么：单条消息内唯一、会话内唯一，还是可用于历史重放。
- 视频第一阶段是否只支持“已有字幕/文本提取”与封面描述，还是需要预留真正的视频时序理解任务。
