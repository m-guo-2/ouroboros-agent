## ADDED Requirements

### Requirement: 全部展开/折叠控制
Decision Inspector SHALL 提供全部展开和全部折叠的控制按钮。

#### Scenario: 点击全部展开
- **WHEN** 用户点击 "全部展开" 按钮
- **THEN** 当前 trace 中所有模型输出、工具调用、工具结果的折叠面板都展开

#### Scenario: 点击全部折叠
- **WHEN** 用户点击 "全部折叠" 按钮
- **THEN** 所有已展开的面板都折叠

### Requirement: 错误 Step 自动展开
Decision Inspector 中的错误类型 step SHALL 默认展开。

#### Scenario: trace 包含错误 step
- **WHEN** trace 的 steps 中存在 `type === "error"` 或 `toolSuccess === false` 的 step
- **THEN** 这些 step 在初始渲染时自动展开，无需用户手动点击

#### Scenario: trace 无错误
- **WHEN** trace 的所有 step 都成功
- **THEN** 所有 step 保持默认折叠

### Requirement: 标签中英文统一
Decision Inspector 中所有 UI 标签 SHALL 统一使用中文。

#### Scenario: Inspector 标题
- **WHEN** Decision Inspector 渲染
- **THEN** 标题显示 "决策详情" 而非 "Decision Inspector"

#### Scenario: Step 类型标签
- **WHEN** 渲染 step 列表中的各类 step
- **THEN** "Model Output" 显示为 "模型输出"（已满足）
- **THEN** "Tool" 显示为 "工具"
- **THEN** "Input" 显示为 "输入"
- **THEN** "Result" 显示为 "结果"
- **THEN** "Error" 显示为 "错误"

### Requirement: Tab Count Badge
Monitor 页面的 tab bar 中每个 tab SHALL 显示对应数据的数量 badge。

#### Scenario: 记忆 tab 有数据
- **WHEN** 当前 session 有 5 条记忆
- **THEN** "记忆" tab 标签旁显示数字 badge "5"

#### Scenario: 定时任务 tab 有数据
- **WHEN** 当前 session 有 3 个定时任务
- **THEN** "定时任务" tab 标签旁显示数字 badge "3"

#### Scenario: Subagent tab 有数据
- **WHEN** 当前 session 有 2 个 subagent job
- **THEN** "Subagent" tab 标签旁显示数字 badge "2"

#### Scenario: tab 无数据
- **WHEN** 某个 tab 对应的数据为空
- **THEN** 不显示 badge（或显示为 0）

#### Scenario: 对话 tab
- **WHEN** 渲染对话 tab
- **THEN** 对话 tab 不显示 badge（消息数量已在 header 中显示）
