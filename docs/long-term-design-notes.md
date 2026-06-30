# 项目长期设计备忘

这份文档从 `openspec/` 中提炼，只保留对项目后续演进仍有价值的信息。`openspec/changes/` 里的任务清单和阶段性方案不应再作为当前实现的事实来源，当前事实以代码、部署配置和运维文档为准。

## 值得长期保留的原则

### Agent 是参与者，不是工具

系统把 human 和 agent 放在统一参与者模型里：Agent 有身份、记忆、渠道和独立判断，不是被人调用后返回结果的函数。

长期约束：
- 人和 Agent 不要设计两套身份体系。
- 同一个群里多个 Agent 应有各自独立的 session、prompt 和 memory。
- 协作逻辑应主要写在 system prompt 和 skills 里，不要硬编码成系统级 workflow。

### 系统只做基础设施

系统负责消息管道、身份、记忆、会话、工具执行和可观测性。角色认知、协作方式、决策逻辑交给 Agent 自己。

长期约束：
- 新增角色优先通过 Agent 配置解决，而不是改核心代码。
- 渠道适配器只做协议转换和资源准备，不向上泄漏平台细节。
- 不把业务协作规则写进 dispatcher、runner 或 channel 层。

### 事件驱动，不是请求响应

Agent 响应的是事件，包括用户消息、定时任务、子 Agent 完成、系统通知等。处理模型应围绕 session 事件流，而不是单条请求。

长期约束：
- 新事件类型应进入统一 session 处理链路。
- 消息处理期间收到的新消息，需要可被当前处理周期感知或持久化恢复。
- 崩溃恢复和队列游标比内存队列更重要。

### 上下文、展示、可观测性分离

三类数据服务不同对象，不能混成一张表：

| 数据 | 服务对象 | 要求 |
|------|----------|------|
| Session Context | LLM | 完整 `AgentMessage[]`，保留 tool_use/tool_result |
| Chat Messages | 用户 | 只保存用户可见内容 |
| Execution Traces | 开发者 | 保存 thought/action/observation 等调试轨迹 |

长期约束：
- 用户可见输出必须由显式发送工具产生。
- 模型自然语言输出是内部思考，不应直接展示给用户。
- Trace 可以重，但不能拖垮主业务存储。

### Admin 所见即所得

Admin 中的 system prompt 应接近最终发给模型的内容。隐式拼接越多，调试越困难。

长期约束：
- `{{skills}}` 这类模板变量可以保留，但必须可预览。
- 不要在代码中偷偷追加大段角色行为规则。
- 如果运行时还会追加系统协议，应在预览或文档中明确。

## 值得保留的架构决策

### Go 单体主进程 + 独立渠道适配器

`agent` 是主 Go 单体，包含 API、Dispatcher、Runner、Engine、Storage 和 Admin 静态托管。`channel-feishu`、`channel-qiwei` 保持独立进程，通过标准化入站/出站契约与主进程交互。

这个边界仍然有价值：主业务状态集中，渠道协议隔离，部署复杂度可控。

### 渠道是管道，Agent 是主体

不要把系统设计成“飞书机器人”或“企微机器人”。飞书、企微、WebUI 都只是同一个 Agent 工作的渠道。

长期约束：
- Channel 只负责平台协议、回调、发送、资源下载/上传。
- Agent 不应该理解平台 raw method 和复杂 payload。
- 新渠道接入应实现适配器，不改核心 Agent 逻辑。

### 企微语义门面

对 Agent 暴露企微能力时，长期方向是少量语义化接口，而不是一堆 raw API：
- 搜索联系人/群
- 读当前或最近会话
- 解析消息/附件
- 发送消息

这样能降低模型工具选择负担，也能把企微细节收口在 `channel-qiwei`。

### 富媒体资源边界

图片、文件、视频、语音等资源不应依赖本地临时路径作为长期边界。稳定方向是先在 channel 层下载并物化到共享 OSS，再向上游暴露最小必要的资源标识。

长期约束：
- 语音适合入口层前置转写，因为文本是语音的基础可消费形态。
- 图片、文件、视频适合按需解析，不要默认全部前置理解。
- Agent 面向附件时应拿结构化附件或稳定资源 URI，而不是正文中的弱链接提示。

### Skill 运行时本地化

运行时 skill 元数据和内容应来自本地 runtime store，而不是依赖 GitHub 内存状态。

长期约束：
- `load_skill`、`load_skill_reference`、`run_script` 应读本地同步后的文件。
- 缺 metadata、缺文件、未绑定、禁用要能区分诊断。
- 同步流程必须先物化必需文件，再让 skill 对运行时可见。

### Skill 两级加载

长期保留 “always + on demand” 思路：
- 常用、身份相关、短内容可以进入 prompt。
- 长文档、低频能力、复杂工具参考应按需 `load_skill`。

原因是 system prompt 每轮都发送，无法被上下文压缩；而按需加载的 tool result 可以进入上下文，被压缩、摘要或丢弃。

### 上下文压缩

长对话不能只硬截断。长期方向是 token 感知压缩：
- 按完整 turn 处理，避免孤儿 tool_use/tool_result。
- 摘要旧上下文，保留近期完整消息。
- 保留可追溯归档或检索入口。
- 子 Agent 压缩不应写入主 session memory，结果应通过 job result 回传。

### Subagent 边界

子 Agent 是任务执行者，不应继承主 Agent 的完整历史，也不应继续启动子 Agent。

长期约束：
- 主 Agent 给子 Agent 的背景应是精炼 context。
- 子 Agent 完成、失败、取消都应有结构化结果。
- 取消时基于已有消息和影响摘要生成进度报告，不额外发起 LLM 调用。

### 可观测性是产品能力

Monitor 不是附属调试页，而是理解 Agent 行为的核心入口。

长期应能看清：
- 用户可见对话
- 决策过程和工具调用
- LLM 输入输出
- 上下文压缩
- 子 Agent job 与 trace
- 定时任务和记忆

## 可以随 `openspec/` 删除的信息

以下内容长期价值低，删除风险小：
- `openspec/changes/*/tasks.md`：阶段性任务清单，容易与当前代码漂移。
- `openspec/changes/*/proposal.md` 和 `design.md`：已实施或废弃后的过程稿，只适合追溯，不适合指导开发。
- `openspec/changes/archive/`：历史变更归档，基本可由 Git 历史替代。
- `openspec/archive/legacy-docs/`：早期 TypeScript server、自举架构、旧渠道设计，多数已经过时。
- `openspec/decisions/` 中偏 UI 小修、一次性 bugfix、运维偏好的 ADR：如 textarea 自动扩高、manifest fallback、防崩修复、显式直推 main 等。
- `openspec/config.yaml` 和 `.openspec.yaml`：只服务 OpenSpec 工作流，删除 OpenSpec 后无长期意义。

## 删除前建议迁出的少量源文档

如果需要保留原始上下文，而不是只保留本备忘，建议只迁出这些文件：
- `openspec/reference/design-principles.md`
- `openspec/reference/architecture.md`
- `openspec/reference/concurrent-message-design.md`
- `openspec/specs/skill-local-runtime-store/spec.md`
- `openspec/specs/shared-oss-storage/spec.md`
- `openspec/specs/subagent-cancelation-progress/spec.md`
- `openspec/specs/subagent-context-compression/spec.md`
- `openspec/specs/subagent-jobs-api/spec.md`
- `openspec/specs/subagent-trace-linking/spec.md`

其他文件不建议迁出，保留成本大于收益。
