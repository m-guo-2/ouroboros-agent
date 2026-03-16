## Context

当前子 agent 的执行入口是 `subagent/manager.go` 的 `run()` 方法，直接调用 `engine.RunAgentLoop` 并在完成后将 `FinalText` 写入 `Job.Result`。主 agent 通过三个回调（`OnCompleted` / `OnFailed` / `OnCanceled`）接收子 agent 结果，回调内部通过 `notifyMain` 将结果包装为合成用户消息入队到主 agent 的事件流。三个回调本质上指向同一个 `notifyMain`，仅靠 `failed bool` 区分格式，语义不够清晰。

现有问题：
1. **取消丢失进度**：`context.Canceled` 时，`run()` 直接调用 `markCanceledWithCallback`，`Job.Result` 为空，主 agent 无法得知子 agent 已完成的工作。
2. **无上下文压缩**：长任务子 agent（如 `data_report` 多轮工具调用）可能超出 token 窗口，没有主 agent 那样的压缩机制。
3. **传入完整历史**：`Start()` 接收 `req.Messages`（主 agent 全部历史），子 agent 实际只需 `task` 描述和少量背景，浪费大量 token 并引入噪音。
4. **可递归嵌套**：子 agent 能看到 `run_subagent_async` 等工具，理论上可以启动子子 agent。

## Goals / Non-Goals

**Goals:**
- 子 agent 所有终态（completed / canceled / failed）通过统一的 event 通道报告给主 agent，消息文本按状态区分语义
- 子 agent 获得与主 agent 一致的上下文压缩能力（外层循环 + LLM 摘要），支持长程任务
- 子 agent 启动时只传入 task + 可选 context hint，不传入主 agent 完整历史
- 从工具列表中排除 subagent 相关工具，阻止递归嵌套

**Non-Goals:**
- 不改变主 agent 的执行循环
- 不为子 agent 引入事件抢占机制（`DrainNewEvents` / `HasNewEvents`）——子 agent 不需要响应外部事件
- 不引入子 agent 间通信
- 不改变 `RunAgentLoop` 的接口

## Decisions

### D1: 取消时构建进度摘要——无额外 LLM 调用

**决策**：当 `RunAgentLoop` 因 `context.Canceled` 返回时，使用已有的 `loopResult.Messages` 和 `Job.Impacts` 拼接一份结构化进度报告，直接填入 `Job.Result`。不发起额外的 LLM 调用来做摘要。

**理由**：
- 取消场景要求快速响应，额外 LLM 调用增加延迟且可能失败
- `Impacts` 已记录了子 agent 执行的每一个工具调用及其结果摘要，信息量充足
- `loopResult.Messages` 中最后一条 assistant 文本（如有）包含子 agent 的最新思考

**替代方案**：
- 发起一次快速 LLM 调用做摘要 → 增加复杂度和延迟，取消场景下 LLM 可能也被取消
- 只返回 Impacts 列表 → 缺少子 agent 的文本输出

**进度报告格式**：
```
[子任务被中断]
已完成的操作：
- <impact 1 summary>
- <impact 2 summary>
...
最后状态：<loopResult 中最后一条 assistant text，截断到 500 字>
```

### D2: 统一回调 `OnDone` + 按状态格式化通知

**决策**：将 `StartRequest` 的三个回调 `OnCompleted` / `OnFailed` / `OnCanceled` 合并为单个 `OnDone func(*Job)`。`run()` 在所有终态（completed / canceled / failed）都调用 `OnDone`。`notifyMain` 根据 `Job.Status` 格式化不同的通知消息，陈述事实，不加引导语。

**通知消息格式**：

| Status | 前缀 | 内容 |
|--------|------|------|
| `completed` | `【subagent完成】` | `subagent={profile} jobId={id}\n\n{result}\n\n已执行操作：\n{impacts}` |
| `canceled` | `【subagent中断】` | `subagent={profile} jobId={id}\n\n{result}\n\n已执行操作：\n{impacts}` |
| `failed` | `【subagent失败】` | `subagent={profile} jobId={id}\n\n错误：{error}\n\n已执行操作：\n{impacts}` |

三种状态走同一个 event 通道，消息前缀明确区分语义。主 agent 自行决定后续操作。

**理由**：
- 三个回调本质指向同一个 `notifyMain`，仅靠 `failed bool` 区分格式，语义模糊
- 子 agent 是主 agent 的"下属"，汇报只陈述事实（做了什么、做到哪、出了什么问题），不指导上级
- `OnDone` 单回调 + `Job.Status` 自描述，调用方不需要选择"该调哪个回调"的问题

**替代方案**：
- 保留三个回调 → 调用方需决定取消走 OnCompleted 还是 OnCanceled，语义纠结
- 取消走 OnCompleted → 语义不准确，canceled 不是 completed

### D3: 子 agent 与主 agent 一致的上下文压缩

**决策**：`run()` 采用与主 agent `processSession` 相同的外层循环模式：`RunAgentLoop` 命中 MaxIterations 后，执行 `CompactContext`（LLM 摘要 + 工具结果截断），带着压缩后的 messages 重新进入 `RunAgentLoop`。MaxIterations 与主 agent 对齐为 25。

**外层循环伪代码**：
```
maxReentries := 3
for reentry := 0; reentry <= maxReentries; reentry++ {
    loopResult = RunAgentLoop(ctx, messages, MaxIterations=25)
    messages = loopResult.Messages

    if loopResult.FinalText != "" || ctx.Err() != nil {
        break  // 正常完成或被取消
    }
    if !loopResult.HitMaxIterations {
        break  // 非迭代上限退出，无需 re-entry
    }

    estimate = EstimateTokens(messages)
    if ShouldCompact(estimate) {
        CompactContext(messages)  // LLM 摘要压缩
    }
    // 带着压缩后的 messages 继续下一轮
}
```

**关键参数**：
- `MaxIterations = 25`：单轮迭代上限，与主 agent 一致
- `maxReentries = 3`：最多 re-entry 3 次，总迭代上限 25×4 = 100 次
- 压缩触发阈值：复用主 agent 的 `triggerRatio = 0.60`
- 压缩目标阈值：复用主 agent 的 `targetRatio = 0.50`

**与主 agent 的差异**：
- **不执行 `FlushMemoryBeforeCompact`**：子 agent 的"记忆"就是最终的 `Job.Result`，不需要写 session memory
- **不检查新事件**：子 agent 无事件队列，外层循环仅由 HitMaxIterations 驱动
- **有总迭代上限**：主 agent 由用户消息驱动，无固定上限；子 agent 用 maxReentries 防止无限运行

**理由**：
- 长程任务（developer 子 agent 做代码修改、data_report 处理复杂数据）需要超过 15 次迭代
- 中间迭代积累的工具结果会膨胀上下文，仅靠 MaxIterations 无法防止 token 溢出
- 复用 `CompactContext` 保持架构一致性，避免子 agent 出现主 agent 已解决的上下文问题

**替代方案**：
- 仅做 `truncateLargeToolResults` 截断 → 不解决上下文累积膨胀问题，长程任务仍会失败
- 不设 maxReentries → 子 agent 可能无限运行，消耗大量 token

### D4: 子 agent 不传入主 agent 历史，但支持可选 context hint

**决策**：`manager.Start()` 不再接收 `req.Messages`，改为新增可选的 `Context string` 字段。`run()` 构造 messages 时：若 `Context` 非空，先插入一条 context 消息，再插入 `taskMessage(req.Task)`。对应地，`run_subagent_async` 工具 schema 新增可选 `context` 参数。

**消息构造逻辑**：
```
无 context:  [taskMessage]
有 context:  [contextMessage, taskMessage]
```

其中 `contextMessage` 格式为：
```
[背景信息]
<context 内容>
```

**理由**：
- 子 agent 的系统 prompt 已包含角色定义和工具使用指南
- `task` 文本由主 agent 编写，应包含子 agent 所需的核心任务描述
- 传入完整历史消耗大量 input tokens，且可能包含无关信息干扰子 agent
- 可选 context 让主 agent 的 LLM 自行决定是否需要补充背景（如用户偏好、先前决策、约束条件），避免子 agent 冗余地重新发现父 agent 已知信息

**业界参考**：
- Claude Code [#4908](https://github.com/anthropics/claude-code/issues/4908) 提出 Scoped Context Passing，选择性继承 summary / git_diff / user_prompt 等片段
- LangChain Deep Agents 采用上下文隔离 + 显式传入策略，子 agent 不自动继承父 agent 的 system prompt、tools 和历史
- 研究表明 LLM 在长上下文中对"中间位置"信息的处理质量显著下降，精炼后的短上下文优于完整历史

**替代方案**：
- 传入完整历史 → token 浪费、注意力退化、噪音干扰
- 纯 task-only 无 context → 安全但可能导致子 agent 缺少战略上下文，需重复工作

### D5: 排除 subagent 工具

**决策**：在 `filterToolsByProfile` 阶段，全局排除 `run_subagent_async` / `get_subagent_status` / `cancel_subagent` 三个工具名，无论 profile 的 `allowedTools` 是否包含它们。

**理由**：
- 比修改每个 profile 的 allowedTools map 更安全，新增 profile 时不会遗漏
- 作为硬约束而非 prompt 约束，不依赖 LLM 遵守

## Risks / Trade-offs

- **[取消进度可能为空]** → 如果子 agent 在第一次 LLM 调用前就被取消，`loopResult` 为 nil 且 `Impacts` 为空。此时进度报告内容为"无可记录操作"，主 agent 可据此判断。Mitigation: 在报告格式中明确标注"子任务尚未开始执行"。

- **[无 LLM 压缩摘要]** → 纯结构化拼接的进度报告不如 LLM 摘要自然。Trade-off: 速度和可靠性优先于可读性；主 agent 的 LLM 会在收到后自行理解和整合。

- **[context hint 质量依赖主 agent LLM]** → 主 agent 可能写出冗余或不足的 context。Mitigation: 在 `run_subagent_async` 的工具描述中给出 context 字段的使用指南（何时需要、写什么），并通过 prompt 引导主 agent 在 task 中写入核心数据和意图、在 context 中补充决策背景。
