## 1. 子 agent 工具隔离

- [x] 1.1 在 `filterToolsByProfile` 中添加全局排除列表，将 `run_subagent_async`、`get_subagent_status`、`cancel_subagent` 从所有 profile 的工具列表中移除
- [x] 1.2 `StartRequest` 结构体：移除 `Messages` 字段，新增 `Context string` 可选字段
- [x] 1.3 `run()` 中构造子 agent 初始消息：若 `req.Context` 非空，先插入 contextMessage（`[背景信息]\n` + context），再插入 `taskMessage(req.Task)`；否则仅 taskMessage
- [x] 1.4 更新 `registerSubagentTools` 函数签名，移除 `messages` 参数，调用侧（`processSession`）同步修改
- [x] 1.5 `run_subagent_async` 工具 schema 新增可选 `context` 参数（type: string，描述：传给子 agent 的背景信息摘要），工具处理函数中读取并传入 `StartRequest.Context`
- [x] 1.6 更新 `run_subagent_async` 工具描述，说明 context 字段的用途和使用建议（何时该传、传什么）

## 2. 取消进度摘要构建

- [x] 2.1 新增 `buildCanceledResult(loopResult *engine.AgentLoopResult, impacts []Impact) string` 函数，从 loopResult.Messages 提取最后一条 assistant 文本（截断 500 字），结合 Impacts summary 列表，拼接结构化进度报告
- [x] 2.2 `StartRequest` 三回调合并为 `OnDone func(*Job)`，`run()` 中所有终态（completed / canceled / failed）统一调用 `OnDone`
- [x] 2.3 重写 `notifyMain`：移除 `failed bool` 参数，根据 `Job.Status` 选择消息前缀（`【subagent完成】` / `【subagent中断】` / `【subagent失败】`），陈述事实不加引导语
- [x] 2.4 修改 `run()` 中 `context.Canceled` 分支：调用 `buildCanceledResult` 构建进度摘要，写入 `Job.Result`，然后调用 `OnDone`
- [x] 2.5 处理 `loopResult` 为 nil 的边界情况：当子 agent 在首次 LLM 调用前就被取消时，生成"尚未开始执行"的进度报告
- [x] 2.6 更新 `registerSubagentTools` 中 `run_subagent_async` 的 `OnCompleted` / `OnFailed` / `OnCanceled` 调用，改为传入单个 `OnDone`

## 3. 子 agent 外层循环与上下文压缩

- [x] 3.1 将 `run()` 中的 `RunAgentLoop` 调用改为外层循环：MaxIterations 改为 25，循环最多 re-entry 3 次（maxReentries=3）
- [x] 3.2 外层循环逻辑：RunAgentLoop 返回后，若 FinalText 非空或 ctx 被取消则 break；若 HitMaxIterations 为 true，执行压缩后 continue
- [x] 3.3 从 `runner` 包导出 `CompactContext`、`EstimateTokens`、`ShouldCompact`、`resolveCompactModel`、`GetContextWindow` 等函数（或将压缩逻辑抽到共享包），使 `subagent` 包可调用
- [x] 3.4 在外层循环中调用 `CompactContext`（跳过 `FlushMemoryBeforeCompact`），需要传入 compactLLM client（从 `req.LLMClient` 构造或新增 `req.CompactLLMClient`）
- [x] 3.5 处理压缩失败的降级：若 `CompactContext` 返回错误，使用 `truncateByFullTurns` 做硬截断后继续

## 4. 清理与验证

- [x] 4.1 编写单元测试：验证 `buildCanceledResult` 在有/无 loopResult、有/无 Impacts 时的输出格式
- [x] 4.2 编写单元测试：验证 `notifyMain` 对三种 Job.Status 生成正确的消息前缀和内容格式
- [x] 4.3 编写单元测试：验证 `filterToolsByProfile` 对所有 profile 均排除 subagent 工具
- [x] 4.4 编写单元测试：验证子 agent 启动时无 context 则 messages 仅包含 taskMessage
- [x] 4.5 编写单元测试：验证子 agent 启动时有 context 则 messages 包含 contextMessage + taskMessage，且 contextMessage 以"[背景信息]"为前缀
- [x] 4.6 编写单元测试：验证外层循环在 HitMaxIterations 时触发压缩并 re-entry
- [x] 4.7 编写单元测试：验证外层循环在达到 maxReentries 后终止
