## ADDED Requirements

### Requirement: Flat event stream in Decision Inspector

Decision Inspector 右侧面板 SHALL 以扁平时间序列展示 trace 事件，取代当前的 Round → Iteration → Step 三层嵌套结构。

事件按 timestamp 升序排列，每行显示事件类型标签和摘要，默认收起，点击展开详情。

#### Scenario: Normal trace with tool calls
- **WHEN** 用户点击一条包含 2 次 iteration 的对话
- **THEN** Decision Inspector SHALL 按时间顺序显示：
  ```
  模型输出: [thinking 摘要]
  工具执行: search_web
  工具结果: [结果摘要]
  模型输出: [thinking 摘要]
  工具执行: read_file
  工具结果: [结果摘要]
  模型输出: [最终回复摘要]
  ```
  不出现 "Iteration 1"、"Iteration 2" 分组标题

#### Scenario: Click to expand model output details
- **WHEN** 用户点击"模型输出"行
- **THEN** SHALL 展开显示：thinking 完整内容、对应 llm_call 的 model/token 统计/耗时/费用、LLM I/O 查看入口

#### Scenario: Click to expand tool details
- **WHEN** 用户点击"工具执行"或"工具结果"行
- **THEN** SHALL 展开显示：工具名称、输入参数（JSON）、返回结果（JSON）、执行耗时、成功/失败状态

#### Scenario: Trace with errors
- **WHEN** trace 中包含 error 类型事件
- **THEN** SHALL 在事件流中显示红色"错误"行，展开可见错误详情

### Requirement: Unified external event label

中间面板的对话时间线 SHALL 使用统一的"外部事件"标签，不再区分"用户"和"系统触发"。

#### Scenario: User-initiated message
- **WHEN** 一条消息由用户主动发送
- **THEN** SHALL 显示为"外部事件"标签，附带消息内容和时间

#### Scenario: System-initiated message
- **WHEN** 一条消息由系统触发（如定时任务、webhook）
- **THEN** SHALL 同样显示为"外部事件"标签，附带实际事件内容，不显示"(系统触发)"占位文本

### Requirement: Preserve absorb round separation

当 trace 包含 absorb 事件（消息吸纳）时，SHALL 保留 Round 标签页切换，每个 Round 内部使用扁平事件流。

#### Scenario: Trace with absorb events
- **WHEN** trace 包含 absorb 事件将步骤分为多个 round
- **THEN** SHALL 在事件流上方显示 Round 标签页（Round 1, Round 2, ...），每个 Round 内部为扁平事件流

#### Scenario: Single round trace
- **WHEN** trace 没有 absorb 事件（只有一个 round）
- **THEN** SHALL 不显示 Round 标签页，直接显示扁平事件流

### Requirement: Compact stats bar retained

TraceStatsBar 和压缩事件（compact）的展示 SHALL 保持不变，位于事件流上方。

#### Scenario: Stats bar display
- **WHEN** Decision Inspector 显示一个 trace
- **THEN** SHALL 在顶部显示 token 统计、迭代次数、费用等摘要信息
