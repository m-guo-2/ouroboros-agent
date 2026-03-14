# 飞书信息查询 Skill

- **日期**：2026-02-10
- **类型**：代码变更
- **状态**：已实施

## 背景

Agent 需要查询飞书群组名称、群成员列表、搜索群组、查询用户信息等，但现有 feishu-agent skill 只暴露了发消息和创建群组的能力，缺少查询类功能。`get_chat_members` 虽在 action.ts 中已注册，但未在 SKILL.md 中记录，agent 无从知晓该能力存在。

## 决策

为飞书服务增加完整的信息查询能力，并以 skill 形式暴露给 agent，让 agent 知道有哪些信息可以获取。

## 变更内容

### 1. channel-feishu 服务层新增 API (`services/message.ts`)

- `listBotChats()` — 列出机器人所在的所有群（自动分页）
- `searchChats(query)` — 按关键词搜索群组
- `batchGetUserId({emails, mobiles})` — 通过邮箱/手机号查找用户 open_id
- `getUserInfo(userId)` — 获取用户详细信息（姓名、部门、工号等）
- `getChatMembers()` 改为自动分页，返回全量成员

### 2. Action 注册 (`routes/action.ts`)

新增 4 个 action：`list_bot_chats`、`search_chats`、`batch_get_user_id`、`get_user_info`

### 3. Skill 定义 (`server/data/skills/feishu-agent/`)

创建 `skill.json` + `README.md`，定义 13 个 tool（含 6 个查询类 tool），通过 skill-manager 编译后注入 agent 上下文。

### 4. Cursor Skill 文档 (`.cursor/skills/feishu-agent/SKILL.md`)

新增"信息查询"能力分类（第 1 节），包含完整的查询场景示例和组合查询工作流。

## 影响

- Agent 现在可以回答"机器人在哪些群里""某个群里都有谁""查一下张三的信息"等查询请求
- 查询群信息需要机器人在群内；查询用户信息需要应用有通讯录读取权限（`contact:user.base:readonly`）
- 搜索群组 API 需要企业管理员开启搜索权限
