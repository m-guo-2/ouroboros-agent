## Why

当前 agent 的 skill 绑定是静态的——配置在 `AgentConfig.Skills` 上，运行时不可变。没有机制在运行时动态加载或卸载 skill。这导致临时性能力（如破冰引导、专题辅助）要么硬编码到主流程里，要么永久挂载白白消耗 prompt token。

本次变更只做**基础设施**：Hook 机制 + 动态加载/卸载能力。具体的业务事件接入（新用户、进群等）后续按需对接。

## What Changes

- 引入 **Hook 机制**：agent 配置中新增 `hooks` 字段，声明 `event → actions` 规则
- Hook 的控制模型是 **Hook 编排式**：hook 是独立的编排点，决定在事件发生时做什么；skill 不声明自己的 lifecycle
- 新增 **`complete_skill` tool**：LLM 主动调用卸载 skill；卸载智能由 skill 的 prompt 内容驱动
- Hook 加载的 skill 直接进入 agent 的统一 skills 列表，运行时不区分"常驻"和"临时"
- **事件名称是开放字符串**，不硬编码事件集合；由调用方（processSession、dispatcher 等）决定在何时以何事件名触发分发
- 第一期不做 `when` 条件过滤、不做 TTL 兜底

## Capabilities

### New Capabilities
- `agent-hooks`: Agent 级别的 Hook 机制——event/actions 规则配置、通用 Hook 分发函数、hook 配置 API
- `ephemeral-skill-lifecycle`: 临时 Skill 的加载与卸载——hook 触发加载、`complete_skill` tool 卸载、运行时 skill 列表的动态合并

### Modified Capabilities
- `skill-local-runtime-store`: skill 运行时解析逻辑需要支持动态加入的 skill ID（hook 触发后追加到 effective skills 列表）

## Impact

- **存储层**：`agent_configs` 表新增 `hooks` JSON 列；新增 `session_active_skills` 表追踪动态加载记录
- **配置 API**：`GET/PUT /api/agents/{id}` 需要支持 `hooks` 字段的读写
- **AgentConfig 类型**：新增 `Hooks []Hook` 字段
- **新增分发函数**：`DispatchHooks(hooks, event, sessionID)` 供任意调用点使用
- **Tool 注册**：新增 `complete_skill` tool，注册到 skill 内部工具集
- **Persona 不参与**：hooks 只在 agent 层定义，persona 不覆盖 hooks
