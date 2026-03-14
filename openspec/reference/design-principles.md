# Moli Agent — 核心设计原则

> 从项目演进中提炼的设计原则。指导所有后续开发决策。

---

## 1. Agent 是参与者，不是工具

Agent 有身份、记忆、判断力、沟通渠道。它不是被调用的函数，它是参与者。与人类同事的唯一区别是 `type: agent`。

**推论**：
- 人和 Agent 共享同一套身份体系（`users` 表，统一参与者模型）
- Agent 能做的事，人也能做，反过来也一样
- 不设计两套体系

---

## 2. 系统只建基础设施，智能交给 Agent

三层架构中，系统只负责消息管道（第一层）和身份记忆（第二层）。第三层——角色认知、协作方式、决策逻辑——完全写在 systemPrompt + skills 里，系统不管。

**推论**：
- 加一个新角色的 Agent，不需要改一行系统代码
- 不在系统层编码业务逻辑或协作流程
- Skill 是 Agent 的能力扩展，不是系统的功能插件

---

## 3. 协作是涌现的，不是编排的

系统不知道 PM 和 Coder 之间有协作关系。PM 的 prompt 写着"把需求发群里"，Coder 的 prompt 写着"看到需求就评估"，协作自然发生。

**推论**：
- 不做 workflow engine 或 agent orchestration
- 一条消息投递给多个 Agent，各自独立决策是否回复
- 消息管道对内容无感知，只管投递

---

## 4. 事件驱动，不是请求-响应

Agent 响应 **事件** 而非仅仅 "用户消息"。事件类型包括：用户消息、定时任务到期、系统通知等。

**推论**：
- 统一工作流：唤醒 Session → 加载 Context → 追加 Event → ReAct Loop → 保存
- Delayed tasks 是一等公民——Agent 可以给自己设定未来的提醒
- 新的事件类型可以不改引擎地接入

---

## 5. 上下文与展示的读写分离

三种数据各自独立，不混为一谈：

| 数据 | 服务对象 | 特征 |
|------|---------|------|
| Session Context | LLM | 完整的 AgentMessage[]，包含 tool_use/tool_result/thinking |
| Chat Messages | 用户 | 只有可见的交互（人类消息 + Agent 显式回复） |
| Execution Traces | 开发者 | 每步 thought/action/observation，大体量异步写入 |

**推论**：
- 不试图用一张表满足所有需求
- Session Context 整体 JSON 覆盖回写，截断以完整回合 (Turn) 为原子单位
- Traces 走独立存储路径（SQLite per-day + JSONL），不拖垮主库

---

## 6. 强制工具回复——Text 是思考，Tool 是行为

模型直接输出的文本是私密的内部思考 (CoT)，用户永远不可见。模型必须且只能调用 `send_channel_message` 才能与用户沟通。

**推论**：
- 消除"反向幻觉"——模型不会误以为心里想的话用户已经看到
- Loop 自然结束条件：模型不再发起 tool call
- 所有用户可见的输出都有明确的 tool_use 记录，完全可追溯

---

## 7. 渠道是管道，Agent 是主体

不是"飞书机器人"或"企微机器人"，而是"PM Agent 通过飞书/企微跟你说话"。一个 Agent 可以绑定多个渠道。

**推论**：
- 所有渠道适配器共享同一个 IncomingMessage / OutgoingMessage 契约
- Agent 进程完全不感知渠道协议细节
- 新渠道接入只需实现适配器，不改 agent 代码

---

## 8. 统一用户身份

同一个人可能通过飞书、企微、WebUI 与 Agent 交流。系统通过影子用户 + 绑定码实现跨渠道的身份统一。

**推论**：
- 记忆按 user_id（非 channel_user_id）存储
- 绑定后自动合并影子用户的数据
- 同一个用户在不同渠道对同一个 Agent 共享记忆和会话

---

## 9. Skill 设计哲学

- **原子性**：每个 Skill 解决一个具体问题，不做瑞士军刀
- **两级加载**：always（全文内联到 system prompt）和 on_demand（仅索引，按需加载）
- **工具自足**：Skill 通过已有工具（shell、http_request）执行，不为每个 Skill 写定制代码
- **Admin 所见即所得**：Admin 写的 system prompt 就是最终 prompt，`{{skills}}` 是唯一的模板变量

---

## 10. 简单、直接、可维护

像 Rob Pike 一样思考：

- 优先清晰的结构和可维护的边界，不追求技巧性实现
- 一个数据库（SQLite）搞定持久化，不过早引入分布式
- 单体 Go 进程 + 独立渠道适配器，职责清晰
- 先定位根因再修复，不在症状处打补丁
