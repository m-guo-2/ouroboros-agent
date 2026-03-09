## Context

这次设计的核心变化，不是“给 subagent 增加浏览器能力”这么简单，而是重新定义浏览器 agent 的交互模型。

旧思路默认模型可以直接猜 CSS selector，然后调用一组 click/type/read/wait 工具完成操作。这种方案实现快，但在真实页面里非常脆弱：选择器不稳定、页面结构变化频繁、调试成本高、模型也缺少一个清晰的“页面心智模型”。

参考 OpenClaw 后，我们决定把浏览器能力拆成三层：
- **运行时层**：给 `browser` subagent 一个隔离、可清理、有限并发的浏览器执行环境
- **交互层**：通过 `snapshot -> ref -> act` 模型让 LLM 以稳定方式理解和操作页面
- **控制流层**：把扫码、验证码、MFA 等人工关卡设计成一等能力，而不是失败分支

当前架构约束仍然有效：
- `subagent.Manager` 通过 profile 控制工具集和 system prompt
- 工具统一以 `types.RegisteredTool` 注册
- 用户通知与回复仍然走现有 channel 体系
- subagent 是有超时与取消语义的 goroutine 任务

## Goals / Non-Goals

**Goals:**
- 为 subagent 增加一个可隔离运行的 `browser` profile
- 提供适合 LLM 的稳定页面交互模型，而不是让模型猜 selector
- 内建人工 checkpoint 机制，支持暂停、通知、恢复、超时
- 将 V1 复杂度控制在“浏览器 worker runtime”级别，而不是完整浏览器平台

**Non-Goals:**
- 不在 V1 支持多标签页编排
- 不在 V1 支持远程桌面接管浏览器
- 不做录制/回放、trace viewer、download/upload 平台化能力
- 不试图绕过 CAPTCHA、MFA 或反爬策略
- 不在 V1 提供通用 CSS selector/action DSL

## Decisions

### D1: 浏览器交互模型采用 `snapshot -> ref -> act`

**选择**：不再把页面操作建模为“传 selector 给 click/type 工具”，而是提供：
- `browser_navigate`
- `browser_snapshot`
- `browser_act`
- `browser_screenshot`
- `request_human_intervention`

其中 `browser_snapshot` 返回：
- 当前页面 URL / title
- 页面文本摘要
- 可交互元素列表
- 每个元素的稳定临时 `ref`
- 元素的语义信息，如 role / name / text / value / disabled

后续动作全部通过 `browser_act(ref, action, payload?)` 执行。

**原因**：
- LLM 更擅长基于结构化页面摘要做决策，而不是发明 selector
- `ref` 调试成本低，失败时可重新 snapshot
- 页面交互模型与后续 observability 更容易对齐

**替代方案**：
- selector 工具集：实现简单，但在复杂页面中不稳定
- 单一 `browser` 巨型工具：能力集中，但 schema 过重，不利于当前 Go 工具注册体系

### D2: V1 保持“小而稳”的工具面

V1 只提供最小必要工具面：
- `browser_navigate(url)`
- `browser_snapshot(mode?)`
- `browser_act(ref, action, value?)`
- `browser_screenshot(full_page?)`
- `request_human_intervention(description, screenshot?, timeout_minutes?)`

`browser_act` 支持的 action 至少包括：
- `click`
- `type`
- `clear`
- `press`
- `select`

**原因**：
- 避免工具面爆炸
- 减少 prompt 教学成本
- 把复杂度集中在快照与 act 路由层，而不是散落到很多 executor

### D3: 浏览器实例与 subagent job 一对一绑定

每个 `browser` subagent 启动时创建自己的浏览器实例和单个 page，上下文、cookie、缓存完全隔离；job 结束时统一清理。

**原因**：
- 会话隔离清晰
- 取消语义容易实现
- 不需要设计共享实例池与资源回收协议

**取舍**：
- 资源占用更高，因此需要全局并发限制

### D4: 人工介入是 checkpoint，不是 fallback exception

`request_human_intervention` 被定义为正常控制流的一部分。典型场景：
- 扫码登录
- 图形验证码
- 短信验证码
- 风控确认
- 需要用户本人判断的页面状态

流程：
1. subagent 调用 `request_human_intervention`
2. 系统可选附带当前截图，向原始渠道发送明确指令
3. subagent 进入等待态
4. 用户在原会话中回复任意消息，或通过 API resolve
5. checkpoint 被解除，subagent 继续执行

**关键决策**：用户回复优先解除 pending checkpoint，而不是直接进入新的 agent 对话。

### D5: V1 不做远程接管浏览器

OpenClaw 的 noVNC / VNC 远程控制方案很强，但它是一个单独的产品层：
- 需要额外鉴权
- 需要暴露访问入口
- 需要处理会话托管与审计
- 需要考虑更大的安全面

因此 V1 先采用“截图 + 文字说明 + 原渠道回复恢复”的轻量 checkpoint 模式。未来如果人工关卡成为高频瓶颈，再在此基础上叠加远程接管能力。

### D6: 运行时安全优先级高于浏览器能力完整性

V1 至少需要以下安全约束：
- `browser` profile 只能访问允许的浏览器工具和人工介入工具
- 禁止访问敏感本地地址与内网地址应有明确防护策略
- 浏览器实例必须在取消和超时场景下可靠回收
- browser subagent 并发数默认限制为 2

## Risks / Trade-offs

- **[快照实现复杂度上升]** -> snapshot/ref 映射比 selector click 更难实现。Mitigation: V1 先输出扁平交互元素列表，不追求完整 accessibility tree。
- **[ref 不稳定]** -> 导航或大幅 DOM 变化后旧 ref 会失效。Mitigation: prompt 中明确要求动作失败后重新 snapshot。
- **[人工 checkpoint 体验有限]** -> 用户只能看截图与说明，不能直接接管浏览器。Mitigation: 将文案和截图做清楚，把远程接管作为 V2。
- **[资源占用]** -> 每个 subagent 独占浏览器进程。Mitigation: 限制并发、使用单 page、及时清理。
- **[安全边界]** -> 浏览器天然能访问更复杂的外部系统。Mitigation: profile 工具白名单 + URL 访问策略 + 默认沙箱运行。

## Migration Plan

1. 先更新 `browser` profile 的 spec 与 prompt 约束
2. 实现浏览器运行时和最小工具面
3. 接入 checkpoint manager 与消息回复拦截
4. 增加并发限制与清理
5. 补齐端到端测试

回滚策略：
- 若 browser profile 不稳定，可临时从 `normalizeProfile` / tool registration 中移除 `browser`
- checkpoint 逻辑可保持独立，不影响现有 developer/file_analysis subagent

## Open Questions

1. `browser_snapshot` 的输出是扁平列表优先，还是树结构优先？我倾向扁平列表优先，便于 LLM 选择。
2. URL 安全策略是直接做 deny private network，还是先只在 prompt + 配置里约束？这决定 V1 的工程复杂度。
3. 是否需要把 checkpoint 状态暴露到 monitor 页面？从运维角度看很有价值，但不是实现 browser runtime 的前置条件。
