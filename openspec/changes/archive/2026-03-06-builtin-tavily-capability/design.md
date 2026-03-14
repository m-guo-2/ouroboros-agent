## Context

当前 Agent 运行时已经有三类外部能力接入路径：builtin tool、skill tool 和 MCP tool。Tavily 的目标场景不是“给某个技能临时加一个联网接口”，而是提供一个主 Agent 和 subagent 都能稳定复用的内置联网研究能力，因此更适合作为 first-party builtin capability 接入。

现有架构约束如下：
- 工具统一通过 `types.RegisteredTool` 注册，并带有 `source` / `sourceName`
- subagent 通过 `profile` 控制 prompt 和可用工具白名单
- 配置通过 `settings` 表统一管理，admin 设置页可直接编辑
- trace / impact 已经是主 Agent 和 subagent 的既有可观测面

这次变更跨越运行时、配置、subagent profile 和 admin 设置，且引入了新的外部依赖，因此需要单独设计文档先把边界定清楚。

## Goals / Non-Goals

**Goals:**
- 为主 Agent 提供可直接调用的 Tavily 内置搜索工具
- 为主 Agent 提供一个适合“联网检索并总结”的内置 research 类 subagent profile
- 将 Tavily 凭据和默认参数纳入现有 settings 管理体系
- 统一 Tavily 输出结构、错误语义和 trace/impact 记录方式
- 保持 V1 简单，只解决“检索 + 总结 + 来源链接”这条主路径

**Non-Goals:**
- 不在 V1 接入 Tavily 的全部接口能力，如 crawl、map、extract 等扩展端点
- 不把 Tavily 包装成新的外部 MCP server
- 不让 `web_research` subagent 获得本地写文件、Shell 等高权限工具
- 不在 V1 做复杂的结果缓存、配额管理或多轮自动事实校验

## Decisions

### D1: Tavily 作为 builtin tool 接入，而不是 MCP/skill

**选择**：新增 Tavily builtin tool，由服务端直接发起 HTTP 调用，并以 `source = "builtin"` 注册到现有 registry。

**原因**：
- 用户诉求是“内置 subAgent 或 tool”，first-party builtin 最符合语义
- 不需要额外部署 MCP server，也不需要把 Tavily 配成 skill 才能使用
- 可以统一配置、审计、错误处理和 trace 记录

**替代方案**：
- MCP server：扩展性好，但增加部署和发现成本，不符合“内置”目标
- Skill HTTP 工具：实现更快，但生命周期和权限边界更偏用户技能，不适合平台级默认能力

### D2: 抽离独立 Tavily client 层，tool 只负责参数校验和结果归一化

**选择**：新增独立的 Tavily client / adapter 层，负责 HTTP 请求、认证、超时和原始响应解析；tool executor 只做输入校验、默认值填充和输出归一化。

**原因**：
- 便于单元测试和后续扩展到更多 Tavily endpoint
- 避免把第三方协议细节散落在 registry/tool 注册逻辑中
- 后续 `web_research` subagent 与主 Agent 共享同一执行路径

**替代方案**：
- 直接在 tool executor 里手写 HTTP 请求：实现简单，但会让注册层承担过多第三方集成细节

### D3: Tavily 配置进入现有 settings 体系

**选择**：Tavily 采用 settings 表管理配置，至少包括：
- `api_key.tavily`
- `base_url.tavily`
- `enabled.tavily`
- 若需要默认参数，可增加 `tavily.search_topic`、`tavily.search_depth`、`tavily.max_results`

admin 设置页展示这些字段，服务端在执行前读取并校验。

**原因**：
- 与现有 provider/settings 管理方式一致
- 便于通过 admin UI 开关 Tavily，而不是改环境变量或代码
- 为后续多环境、运维和故障排查保留统一入口

**替代方案**：
- 仅用环境变量：实现方便，但与当前产品化配置流不一致
- 写死默认 URL/开关：可运行，但缺乏运营和故障切换能力

### D4: `web_research` subagent profile 是对 Tavily tool 的受限包装

**选择**：新增 `web_research` profile，默认只允许使用 Tavily 搜索能力和必要的上下文回忆能力，不开放 Shell、写文件等高权限工具。

**原因**：
- 用户任务里“联网研究”与“本地改代码”是两种不同权限模型
- 受限工具面更容易写 prompt，也更安全
- 可以让主 Agent 明确把外部资料收集任务委派给专用子代理

**替代方案**：
- 只做 tool、不做 subagent：实现更小，但无法满足“内置 subAgent”诉求
- 复用 `developer` profile：灵活，但权限过大，职责边界不清晰

### D5: V1 统一返回精简结构，并记录轻量 observability

**选择**：`tavily_search` 返回精简且稳定的结构，至少包含：
- `query`
- `answer` 或摘要字段
- `results[]`，每项包含 `title`、`url`、`snippet`、`score`
- `total_results`
- `returned_results`
- `truncated`
- `response_time_ms`（若可获得）

默认只返回有限数量的高相关结果，且每条结果的文本片段也必须截断到固定预算内；tool 不透传 Tavily 原始长文本，也不把全部命中结果直接送进模型上下文。

trace / impact 只记录查询词、命中总数、实际返回数、是否截断、是否命中错误和少量元数据，不直接落完整原始网页内容。

**原因**：
- LLM 需要稳定结构，避免绑定 Tavily 原始 JSON 的细节
- Web search 命中结果和单条内容都可能很多，必须在进入上下文前先做预算控制
- 降低日志噪音和潜在敏感数据泄露风险
- 便于后续在 monitor 页面做结构化展示

**替代方案**：
- 透传完整 Tavily 原始响应：最省事，但会加大 prompt 噪音并增加协议耦合

### D6: `web_research` 采用迭代式检索，而不是一次吞下全部搜索结果

**选择**：`web_research` profile 的 prompt 和执行约束必须显式鼓励“先取少量高相关结果 -> 判断是否足够 -> 不足再缩窄或改写 query 继续检索”，而不是一次请求大量结果后原样汇总。

**原因**：
- 联网研究的核心瓶颈通常不是“搜不到”，而是“结果太多”
- 迭代式检索更符合 token 预算，也更利于主 Agent 获得高信噪比结论
- 这能把“检索策略”从 Tavily API 细节中抽离出来，形成稳定的 subagent 行为约束

**替代方案**：
- 单次大结果集检索：实现简单，但容易把无关内容和长片段一起塞进上下文
- 完全由主 Agent 自己决定分页：更灵活，但会让 research profile 失去明确价值

## Risks / Trade-offs

- **[新增外部依赖]** -> Tavily 不可用时会影响联网研究能力。Mitigation: 配置级启停、明确错误提示、对主流程保持可降级。
- **[结果质量受第三方影响]** -> 搜索结果可能噪声较大或摘要不稳定。Mitigation: V1 输出来源链接和结果列表，让主 Agent/subagent自行综合。
- **[结果数量过多]** -> 单次检索可能命中大量 URL，单条内容也可能很长。Mitigation: 默认限制返回条数、截断 snippet，并暴露 `truncated` / `total_results` 给调用方。
- **[subagent 能力重复]** -> 既有 tool 又有 research profile，可能看起来重叠。Mitigation: 明确“tool 给主 Agent 直接用，profile 给委派研究任务用”。
- **[配置项增加]** -> settings/admin UI 会新增 Tavily 字段。Mitigation: 保持字段最小集，默认只要求 API Key，Base URL 使用默认值。
- **[trace 泄露风险]** -> 外部检索词可能包含敏感上下文。Mitigation: 仅记录必要元数据，并遵循现有敏感信息清洗策略。

## Migration Plan

1. 新增 Tavily client 和 builtin tool 注册路径
2. 接入 settings 读取、默认值和未配置错误处理
3. 扩展 `subagent.Manager`，增加 `web_research` profile、白名单和 prompt
4. 在 admin settings 页面增加 Tavily 配置项
5. 补齐 tool、profile 和 API 配置相关测试
6. 验证结果截断、返回计数和迭代式检索约束符合预期

回滚策略：
- 若 Tavily tool 不稳定，可先从 registry 中移除注册，不影响其他 builtin / skill / MCP 工具
- 若 `web_research` profile 体验不佳，可仅保留 Tavily tool，对外暂不暴露 research subagent

## Open Questions

1. V1 是否需要把 Tavily 的 `topic`、`search_depth` 暴露给最终工具输入，还是先由服务端固定默认值？
2. `web_research` profile 是否只允许 Tavily + recall，还是需要只读 `read_file` 用于结合本地上下文写总结？
3. `max_results` 与 `snippet` 长度上限是完全固定，还是允许在受控上限内由调用方显式指定？
4. monitor / trace 页面是否要在本次变更中直接展示 Tavily 查询和来源列表，还是先只落后端 observability？
