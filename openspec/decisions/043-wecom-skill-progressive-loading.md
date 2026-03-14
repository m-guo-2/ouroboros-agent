# 企微 Skill 渐进式加载与工具合并

- **日期**：2026-03-04
- **类型**：架构决策 / 代码变更
- **状态**：待实施

## 背景

moli（企微 agent）接入 channel-qiwei 后，面临三个问题：

1. **工具爆炸**：channel-qiwei 有 10 个模块、92 个 action。按原有模式每个 action 注册为一个 tool，会严重影响模型选择准确率（Anthropic 官方指出超过 30-50 个 tool 后准确率显著下降）
2. **SystemPrompt 膨胀不可压缩**：所有 skill 的 readme 和 tool 定义放在 SystemPrompt 中，compact 机制无法对其优化，每次请求都消耗固定 token 预算
3. **上下文干扰**：一次性注入大量无关 skill 信息会分散模型注意力，降低输出质量
4. **per-agent skill 绑定缺失**：`GetSkillsContext` 忽略 agentID，所有 agent 共享全部 enabled skill

参考 Anthropic 官方指导：
- [Writing tools for agents](https://www.anthropic.com/engineering/writing-tools-for-agents)："Consolidate related operations into fewer tools with an action parameter"
- [Tool Search Tool](https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/tool-search-tool)：`defer_loading` 机制，按需发现和加载 tool，token 减少 85%

## 决策

### 1. 内置 SystemPrompt 瘦身

代码中 `buildSystemPrompt` 的 builtin 部分从完整的"消息回复与输出原则"缩减为仅保留**消息格式协议**（~200 tokens）。角色定位、回复行为、输出原则等全部移至数据库 `agent_configs.system_prompt`，由各 agent 独立配置。

保留的 builtin 仅描述消息历史的数据结构格式，这是 runner 层面的协议，与 agent 个性无关。

### 2. 工具合并

遵循 Anthropic "fewer, more capable tools" 原则，将 92 个 action 合并为：

**永久加载（6 个 tool）：**

| Tool | 说明 |
|------|------|
| `send_channel_message` | 回复当前会话用户（已有内置工具） |
| `wecom_send_message` | 发消息，type 参数区分 text/image/file/link 等 |
| `wecom_search_contact` | 搜索联系人、获取联系人详情 |
| `wecom_list_groups` | 查询群列表、获取群详情 |
| `wecom_api` | 通用 API 透传，接受 method + params |
| `load_skill` | 加载扩展技能文档和工具参考（内置） |

**按需加载（通过 load_skill 获取文档后，用 wecom_api 执行）：**

| Skill ID | 覆盖场景 |
|----------|----------|
| `wecom-group-mgmt` | 建群、改名、加/踢成员、群公告、转让群主等 |
| `wecom-contact-mgmt` | 添加好友、通过申请、修改备注、删除联系人等 |
| `wecom-message-mgmt` | 撤回、置顶、群发、同步历史消息 |
| `wecom-moment` | 朋友圈：浏览、发布、点赞、评论 |
| `wecom-cdn` | 文件上传下载、CDN 转链 |
| `wecom-tag` | 标签管理 |
| `wecom-session` | 会话列表与分组管理 |

**不暴露给 agent：** Login、Instance、Logout（基础设施/运维操作）。

### 3. 渐进式加载（load_skill）

增强原有 `get_skill_doc` 为 `load_skill`，作为内置工具：

- **输入**：`skill_id`（技能 ID）
- **返回**：skill 的完整文档（readme）+ 所含工具的 method/params 参考
- **模型行为**：读取文档后，通过 `wecom_api` 万能工具调用具体 method

**关键优势**：加载的内容进入普通 context（tool_result），后续 compact 可以自然压缩；而 SystemPrompt 中的内容永远不会被 compact 优化。

### 4. per-agent skill 绑定

激活 `agent_configs.skills` 字段（当前已有但未使用）：

- `skills` 非空时：仅加载列表中的 skill 的 tools 和 readme
- `skills` 为空时：加载全部 enabled skill（向后兼容）
- `load_skill` 可以访问所有 enabled skill（不受 agent 绑定限制）
- SystemPrompt 附加段中，对未绑定但可用的 skill 给出简短索引

## 变更内容

| 文件 | 改动 |
|------|------|
| `agent/internal/runner/processor.go` | builtin 缩减为消息格式协议；`get_skill_doc` → `load_skill`；传递 agentSkills |
| `agent/internal/storage/skills.go` | `GetSkillsContext` 接受 `agentSkills` 参数，区分 active/deferred skill |
| DB: `skills` 表 | 新增 `wecom-core` + 7 个按需 skill 记录 |
| DB: `agent_configs` | 更新 moli 的 `system_prompt` 和 `skills` 字段 |

## 考虑过的替代方案

1. **全部注册为独立 tool**：92 个 tool 远超 Anthropic 建议的上限，准确率和 token 开销不可接受
2. **全部放 SystemPrompt**：不可被 compact，永久占用 token 预算
3. **动态注册工具到 registry**：需改 engine loop 每轮重读 tool 列表，过度复杂。文档加载 + 万能 API 透传更简洁

## 影响

- moli 的 SystemPrompt 应迁移至数据库，包含角色定位和回复原则
- 新增的 skill 数据需通过 SQL 或 Admin UI 写入数据库
- 其他 agent（如飞书）不受影响，`skills` 为空时行为与当前一致
- 后续 channel-feishu 可参照同样模式做 skill 合并
