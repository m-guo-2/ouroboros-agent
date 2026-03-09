# Skill 设计哲学与核心理念

- **日期**：2026-03-04
- **类型**：架构设计 / 设计规范
- **状态**：已确认

## 核心矛盾

Agent 的 Skill 系统面对一个根本性的张力：

- **上下文充足**才能让模型准确执行——模型需要知道自己有什么能力、每个工具的参数是什么、在什么场景下使用。上下文不足会导致**幻觉**（hallucination）：模型臆造不存在的工具，或错误组合参数。
- **上下文过载**会降低模型表现——Anthropic 官方指出超过 30-50 个 tool 后准确率显著下降。过多的 skill 文档注入 SystemPrompt 会挤占有效对话空间，且 SystemPrompt 不可被 context compaction 压缩。

**这不是可以一次性解决的问题，而是需要在架构中持续管理的张力。** 以下原则都是围绕这个核心矛盾展开的。

---

## 原则一：Skill 是能力的原子单位

### 什么是 Skill

Skill 不是单个工具，也不是一段文档。Skill 是 Agent 拥有的一项**完整能力**，它包含三个层面：

| 层面 | 作用 | 类比 |
|------|------|------|
| **Tools（行动）** | 可调用的工具定义，含 inputSchema 和执行器 | 手 |
| **Readme（知识）** | 使用指南、工作流说明、注意事项 | 脑 |
| **Description（身份）** | 一句话描述这项能力是什么 | 名片 |

**关键约束：这三个层面必须作为一个整体存在、一个整体加载。** 不能出现"知道怎么用但没有工具可调"或"有工具但不知道什么时候该用"的状态。

### 为什么是原子的

MCP（Model Context Protocol）将 Tools、Resources、Prompts 做了访问方式上的区分：

- Tools：模型通过 tool_use 主动调用
- Resources：上下文注入，模型被动读取
- Prompts：预置的对话模板

但这是**访问方式的区分，不是存储或组织方式的区分**。一个 Skill 的 tools 和 readme 虽然通过不同方式传递给模型（一个是工具列表，一个是 SystemPrompt 文本），它们在**存储、版本控制、加载/卸载**的粒度上必须是同一个单元。

这解决了 MCP 架构下 skill 描述与工具不强耦合的问题——当 `load_skill` 加载一个 skill 时，它的 tools 和 readme **同时到达**，不存在部分加载导致的不一致。

---

## 原则二：两级加载，按需展开

### 问题

以企微 agent 为例，channel-qiwei 有 10 个模块、92 个 action。全部注册为 tool 不现实（工具准确率雪崩），全部写进 SystemPrompt 也不可行（不可压缩的 token 黑洞）。

### 解决方案：System Loaded + On-Demand Loaded

```
┌─────────────────────────────────────────────────┐
│                  Agent 启动时                      │
│                                                   │
│  System Loaded Skills（系统加载）                    │
│  ├── tools → 注册为可调用工具                        │
│  ├── readme → 注入 SystemPrompt                    │
│  └── 选择标准：高频、核心、agent 身份必需              │
│                                                    │
│  On-Demand Skills（按需加载）                        │
│  ├── 仅展示 id + name + description（索引）          │
│  ├── tools/readme 不加载                            │
│  └── 通过 load_skill 按需获取                        │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│              用户说"帮我建个群"                      │
│                                                   │
│  模型判断 → 需要群管理能力 → 调用 load_skill          │
│  ├── 返回 readme（工作流 + 注意事项）                 │
│  ├── 返回 tools（每个工具的完整定义）                  │
│  └── 进入 tool_result，可被 compact 压缩             │  ← 关键优势
│                                                    │
│  模型读取 → 选择具体工具 → 执行                       │
└─────────────────────────────────────────────────┘
```

### 为什么 load_skill 返回的内容可以被 compact

SystemPrompt 在每次 LLM 调用中都会被完整发送，context compaction 无法对其优化。而 `load_skill` 的返回值作为 `tool_result` 进入对话历史——当上下文窗口不够时，compaction 可以将其摘要或丢弃，因为模型已经"学到"了需要的信息并完成了调用。

### 分类决策标准

| 判断维度 | System Loaded | On-Demand |
|----------|--------------|-----------|
| 使用频率 | 几乎每次对话都用 | 特定场景才用 |
| 身份相关性 | 定义了 agent 是谁 | 扩展了 agent 能干什么 |
| 工具数量 | 少（≤6 个 tool） | 可以多（每个 skill 内部 3-20 个 tool） |
| token 敏感度 | 可接受固定开销 | 需要按需控制 |

---

## 原则三：工具定义要自足

### 核心标准

一个好的工具定义，模型**仅凭 tool name + description + inputSchema 就能正确调用**，不需要额外阅读文档。

### 反面案例（我们曾经犯的错）

```json
{
  "name": "session_getSessionPage",
  "description": "获取会话列表",
  "inputSchema": {
    "properties": {
      "pageNum": { "type": "integer" },
      "pageSize": { "type": "integer" }
    }
  }
}
```

问题：
- description 没有说清返回什么、用在什么场景
- 参数没有 description、没有默认值提示、没有约束说明
- 模型不知道 `pageNum` 从 0 还是从 1 开始

### 正面案例（当前标准）

```json
{
  "name": "get_session_page",
  "description": "分页查询会话列表。返回包含最近聊天对象（个人/群）的会话摘要。每页默认 20 条，按最后消息时间倒序。用于了解最近的沟通对象和消息概况。通过 wecom_api 调用，method: /session/getSessionPage。",
  "inputSchema": {
    "type": "object",
    "properties": {
      "pageNum": {
        "type": "integer",
        "description": "页码，从 1 开始。默认 1",
        "default": 1
      },
      "pageSize": {
        "type": "integer",
        "description": "每页条数，1-100。默认 20",
        "default": 20
      }
    }
  }
}
```

### 自足性检查清单

1. **description 3-4 句**：做什么 → 返回什么 → 典型使用场景 → 注意事项
2. **每个参数有 description**：含类型约束、取值范围、默认值
3. **关联工具有提示**：如"可通过 wecom_search_contact 获取用户 ID"
4. **required 准确标注**：必填和选填严格区分

---

## 原则四：README 是工作流指南，不是 API 手册

### 分工

| | 工具定义 (tools) | README (readme) |
|---|---|---|
| 受众 | 模型的工具调用器 | 模型的推理引擎 |
| 内容 | 单个工具的精确规格 | 多个工具的组合使用方法 |
| 格式 | JSON Schema | 自然语言 + 示例 |
| 更新频率 | 随 API 变化 | 随业务流程变化 |

### README 应该写什么

```markdown
## 群组管理

### 常用工作流

**建群并配置**
1. `create_room` 建群（至少 2 人）
2. `set_room_name` 设置群名
3. `set_room_announcement` 设置公告
4. `invite_room_member` 邀请更多成员

### 关键注意事项
- 群主才能修改群名和公告
- 踢人操作不可撤销
- roomId 在建群时返回，后续所有群操作都需要它
```

### README 不应该写什么

不要把每个工具的参数再抄一遍。那是 inputSchema 的职责。README 的价值在于**工具之间的串联逻辑**和**业务层面的约束**。

---

## 原则五：防止幻觉的三道防线

### 第一道：Skill 原子加载

`load_skill` 返回 tools + readme 作为一个整体。模型看到的工具引用和实际可调用的工具来自同一个数据源。不存在"README 提到了工具 A，但工具 A 没注册"的情况。

### 第二道：工具定义自足

即使 README 在 compact 过程中被压缩或丢弃，工具定义（作为 tool 列表的一部分）仍然完整。模型可以仅凭 tool 定义正确调用，不依赖 README 存活。

### 第三道：索引层保底

所有 On-Demand Skill 在 SystemPrompt 中有一行索引（id + name + description）。模型不会"忘记"这些能力的存在，但也不会被完整定义淹没。当需要时，它知道该调用 `load_skill`。

```
┌────────────────────────────────────────────┐
│  SystemPrompt（不可压缩，始终存在）             │
│  ├── Agent 角色定义                          │
│  ├── System Loaded Skills（tools + readme）  │
│  └── On-Demand Skills 索引（一行摘要）         │ ← 防线三
├────────────────────────────────────────────┤
│  Tool 列表（不可压缩，始终存在）                │
│  ├── System Loaded 的 tools                  │
│  └── load_skill 工具本身                      │ ← 防线二
├────────────────────────────────────────────┤
│  对话历史（可被 compact 压缩）                  │
│  └── load_skill 的 tool_result               │
│      ├── readme（可被压缩）                    │
│      └── tools 定义参考（可被压缩）             │ ← 防线一
└────────────────────────────────────────────┘
```

---

## 原则六：Per-Agent Skill 绑定

### 问题

不同 Agent 有不同的能力范围。企微 agent 不需要飞书的 skill，反之亦然。

### 机制

Agent 配置中的 `skills` 字段决定了 System Loaded 的范围：

- `skills` 非空 → 仅列表中的 skill 被 System Loaded
- `skills` 为空 → 所有 enabled skill 被 System Loaded（向后兼容）
- `load_skill` 可以访问所有 enabled skill，不受绑定限制

这意味着 `skills` 字段控制的是"默认激活"，而不是"允许访问"。Agent 始终可以通过 `load_skill` 发现和加载任何 enabled skill。

---

## 原则七：工具合并优于工具增殖

### Anthropic 的指导

> "Consolidate related operations into fewer tools with an action parameter."
> — [Writing tools for agents](https://www.anthropic.com/engineering/writing-tools-for-agents)

### 在我们系统中的实践

channel-qiwei 的 92 个 action 没有注册为 92 个 tool。而是：

| 策略 | 示例 | 效果 |
|------|------|------|
| **合并同类操作** | `send_message(type: text\|image\|file\|link)` 替代 4 个独立 tool | 4 → 1 |
| **保留万能透传** | `wecom_api(method, params)` 覆盖所有长尾操作 | 兜底 |
| **分层加载** | 群管理 18 个 action 在 On-Demand Skill 中按需展开 | 平时 0 token |

最终 System Loaded 仅 6 个 tool，加上 `load_skill` 共 7 个。模型在大多数对话中只需要面对 7 个工具的选择空间。

---

## 原则八：执行路径要短

### 演进历程

```
v1: 模型 → 读 README → 手动拼 curl → shell 执行
    问题：容易拼错 URL/参数，出错率高

v2: 模型 → 读 README → 构造 method+params → wecom_api 执行
    改进：不用拼 curl，但仍依赖 README 获知参数格式

v3: 模型 → 看 tool 定义（inputSchema）→ 直接 tool_use 调用
    目标：工具定义即合约，模型无需额外阅读就能准确调用
```

当前系统处于 v2 → v3 的过渡期。System Loaded 的核心工具已经是 v3（有完整 inputSchema 的独立 tool）。On-Demand Skill 中的工具定义也已升级到 v3 标准，但执行仍通过 `wecom_api` 透传（因为动态注册 tool 到 engine loop 的改造尚未完成）。

### 最终目标

`load_skill` 加载后，skill 的 tools 应该**动态注册**为可直接 tool_use 调用的工具，而不是让模型"读完文档后手动通过 wecom_api 调用"。这样：

1. 模型可以直接 `tool_use: create_room`，而不是 `tool_use: wecom_api, method: "/room/createRoom"`
2. 减少一层间接，降低出错概率
3. inputSchema 由框架校验，而不是靠模型自觉遵守

---

## 设计决策记录

| 决策 | 选择 | 理由 |
|------|------|------|
| Skill 组织粒度 | 按能力域划分（群管理、联系人等） | 一个 skill 内的 tools 高内聚，跨 skill 低耦合 |
| System Loaded 数量 | ≤ 6 个 tool + load_skill | Anthropic 建议工具总数控制在合理范围 |
| On-Demand 加载方式 | load_skill 内置工具 | 进入 tool_result 可被 compact，不占 SystemPrompt |
| Tool description 长度 | 3-4 句 | 自足但不冗余 |
| README 定位 | 工作流指南 | 不重复 tool 定义，专注于串联逻辑 |
| 执行器 | wecom_api 透传（过渡）→ 动态注册（目标） | 减少间接层，提高准确率 |
| 存储 | 数据库（当前）→ GitHub 仓库 + 文件系统（规划） | 版本控制、协作、可审计 |
| 基础设施操作 | 不暴露给 Agent | Login/Instance/Logout 是运维行为，不是用户能力 |

## 影响

- 新增 Skill 时，必须同时定义 tools 和 readme，不允许只有其中一个
- 工具定义的 description 和 inputSchema 必须通过自足性检查
- README 不重复参数说明，专注于工作流和业务约束
- 后续 channel-feishu 等其他渠道的 skill 迁移遵循相同原则
