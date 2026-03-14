## Why

当前系统的内置能力主要覆盖本地文件、Shell、skill 和 MCP，但缺少一个“无需额外部署即可直接访问实时网页信息”的一等能力。接入 Tavily 可以让主 Agent 和 subagent 在需要最新外部信息、网页摘要和来源链接时走统一平台能力，而不是依赖临时脚本或额外 MCP 配置。

## What Changes

- 新增基于 Tavily API 的内置搜索能力，作为平台级 builtin tool `tavily_search` 暴露给主 Agent
- 为 Tavily 增加基础配置项，包括 API Key、Base URL、启用开关和默认搜索参数
- 新增面向网络研究任务的内置 subagent profile `web_research`，作为 `tavily_search` 的受限包装，使主 Agent 可以把“检索并总结外部资料”委派给专用子代理
- 统一 Tavily 返回结构，至少包含 query、摘要/answer、结果列表、来源 URL，以及 `total_results`、`returned_results`、`truncated`
- 对 Web Search 结果引入预算控制，包括默认结果上限、单条 snippet 截断，以及大量结果时的迭代式缩窄检索策略
- 为调用失败、未配置密钥、被禁用等场景提供明确错误反馈，并在 trace/impact 中保留可观测信息

## Capabilities

### New Capabilities
- `tavily-search-tool`: 基于 Tavily API 的内置 Web Search tool，支持主 Agent 直接发起联网检索，并以受控预算返回结构化结果
- `web-research-subagent`: 面向联网研究任务的内置 subagent profile，作为 `tavily_search` 的受限包装，限制工具面并优先通过 Tavily 完成外部资料收集与总结

### Modified Capabilities

（无已有 spec 变更）

## Impact

- **agent/internal/engine/**: 新增 Tavily builtin tool 注册与执行逻辑
- **agent/internal/subagent/**: 扩展新的 research 类 profile、工具白名单和 system prompt
- **agent/internal/storage/** 与 **agent/internal/api/**: 增加 Tavily 配置读取与管理接口
- **admin settings UI**: 增加 Tavily 配置入口与启用状态管理
- **agent 上下文预算**: 需要对高量级搜索结果做裁剪、摘要和截断元数据暴露
- **外部依赖**: 引入 Tavily HTTP API 作为新的可选联网依赖
