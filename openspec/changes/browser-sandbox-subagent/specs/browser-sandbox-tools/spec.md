## ADDED Requirements

### Requirement: Browser profile subagent support
系统 SHALL 支持 `browser` 作为合法的 subagent profile。当主 agent 调用 `run_subagent_async` 且 `profile` 为 `"browser"` 时，系统 SHALL 创建一个拥有浏览器运行时与专用工具集的 subagent。

#### Scenario: 启动 browser subagent
- **WHEN** 主 agent 调用 `run_subagent_async` 且 `profile` 为 `"browser"`
- **THEN** 系统创建 subagent job，状态从 `queued` 进入 `running`
- **AND** 该 subagent 仅拥有 `browser_navigate`、`browser_snapshot`、`browser_act`、`browser_screenshot` 和 `request_human_intervention` 工具

#### Scenario: 非法 profile 拒绝
- **WHEN** `run_subagent_async` 的 `profile` 为未知值
- **THEN** 系统返回错误 `"unsupported subagent profile"`

### Requirement: Browser runtime lifecycle
系统 SHALL 在 browser subagent 启动时创建独立的浏览器实例与单个 page，并在 subagent 结束、失败、超时或取消时自动销毁。不同 browser subagent 之间 SHALL NOT 共享浏览器实例。

#### Scenario: 正常生命周期
- **WHEN** browser subagent 开始运行
- **THEN** 系统创建新的 headless browser 实例和 page
- **AND** subagent 结束后对应浏览器进程被关闭

#### Scenario: 取消时清理
- **WHEN** browser subagent 运行中被取消
- **THEN** 浏览器实例立即进入清理流程
- **AND** 不保留活动 page 或 orphan 进程

### Requirement: browser_navigate tool
系统 SHALL 提供 `browser_navigate` 工具导航到指定 URL。

输入:
- `url` (string, required): 目标地址

输出: JSON 对象，至少包含 `url`、`title`。

#### Scenario: 导航成功
- **WHEN** subagent 调用 `browser_navigate` 且 URL 有效
- **THEN** 浏览器跳转到目标页面
- **AND** 工具返回最终 URL 和页面标题

#### Scenario: 导航失败
- **WHEN** subagent 调用 `browser_navigate` 且 URL 非法或访问失败
- **THEN** 工具返回错误信息

### Requirement: browser_snapshot tool
系统 SHALL 提供 `browser_snapshot` 工具，用于向模型返回稳定的页面理解结果，而不是原始 DOM 或 CSS selector。

输出 SHALL 包含:
- 当前页面 URL
- 当前页面标题
- 页面文本摘要
- 可交互元素列表
- 每个元素的临时 `ref`
- 每个元素的最少语义字段：`role`、`name`、`text` 中的可用子集

系统 MAY 提供不同 snapshot 模式，但 V1 至少 SHALL 支持一个默认模式。

#### Scenario: 获取页面快照
- **WHEN** subagent 调用 `browser_snapshot`
- **THEN** 系统返回结构化页面快照和一组可用于后续动作的 `ref`

#### Scenario: 页面发生变化后重新快照
- **WHEN** 页面导航或 DOM 发生明显变化，旧 `ref` 不再有效
- **THEN** subagent 可以重新调用 `browser_snapshot`
- **AND** 系统返回新的 `ref` 集合

### Requirement: browser_act tool
系统 SHALL 提供 `browser_act` 工具，使用 `ref` 而不是 CSS selector 对页面执行动作。

输入:
- `ref` (string, required): 来自最近一次 `browser_snapshot` 的元素引用
- `action` (string, required): 动作类型
- `value` (string, optional): 动作附带值

V1 SHALL 至少支持以下 action:
- `click`
- `type`
- `clear`
- `press`
- `select`

#### Scenario: 通过 ref 点击元素
- **WHEN** subagent 调用 `browser_act` 且 `action` 为 `"click"`，`ref` 对应可点击元素
- **THEN** 系统执行点击并返回成功结果

#### Scenario: 通过 ref 输入文本
- **WHEN** subagent 调用 `browser_act` 且 `action` 为 `"type"`，并提供 `value`
- **THEN** 系统在对应元素中输入文本

#### Scenario: ref 失效
- **WHEN** subagent 调用 `browser_act` 使用了已失效的 `ref`
- **THEN** 工具返回明确错误
- **AND** 错误信息提示应重新调用 `browser_snapshot`

### Requirement: browser_screenshot tool
系统 SHALL 提供 `browser_screenshot` 工具，用于获取当前页面截图，服务于调试、观测与人工 checkpoint。

输入:
- `full_page` (bool, optional, default false): 是否截取整页

输出: JSON 对象，至少包含 `image_base64`。

#### Scenario: 获取当前截图
- **WHEN** subagent 调用 `browser_screenshot`
- **THEN** 系统返回当前页面 PNG 截图

### Requirement: Browser profile prompt constraints
系统 SHALL 为 `browser` profile 提供专用 system prompt，至少包含以下行为约束：
- 优先使用 `browser_snapshot` 理解页面
- 使用 `browser_act` 而非猜测 selector
- 在扫码、验证码、MFA、人眼判断等场景调用 `request_human_intervention`
- 在动作失败且怀疑页面变化时重新 snapshot

#### Scenario: browser subagent 启动
- **WHEN** browser subagent 启动
- **THEN** 其 system prompt 包含 snapshot-first 的浏览器操作指引
