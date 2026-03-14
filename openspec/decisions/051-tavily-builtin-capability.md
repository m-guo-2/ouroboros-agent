# Tavily 内置联网研究能力

- **日期**：2026-03-06
- **类型**：架构决策
- **状态**：已实施

## 背景

当前系统已经支持 builtin tool、skill 和 MCP，但缺少一个无需额外部署即可直接访问实时网页信息的一等能力。用户希望把 Tavily 接入成平台内置的 `subAgent` 或 `tool`，用于外部资料检索、摘要和来源引用，而不是依赖临时脚本或额外 MCP 配置。

## 决策

Tavily 采用平台内置能力接入：一方面提供 `tavily_search` builtin tool 给主 Agent 直接调用；另一方面新增受限的 `web_research` subagent profile，作为对 Tavily 搜索能力的内置包装。

## 变更内容

- 在 OpenSpec 中新增 `builtin-tavily-capability` change，包含 `proposal`、`design`、`tasks` 和两个 capability spec。
- 明确 Tavily 走 first-party builtin 路径，而不是外置 MCP server 或普通 skill。
- 明确 Tavily 配置进入现有 `settings` / admin settings 体系，包括 API Key、Base URL、启用开关与默认参数。
- 明确 `web_research` profile 只开放 Tavily 与必要低风险上下文能力，不开放 Shell 与文件写入。
- 明确 V1 先支持“检索 + 摘要 + 来源链接”，不扩展到 Tavily 全量 endpoint。
- 明确 web search 结果进入上下文前必须经过预算控制，包括结果条数上限、snippet 截断，以及 `total_results` / `returned_results` / `truncated` 等元数据。
- 明确 `web_research` profile 在结果过多时应优先缩窄检索，而不是一次性吞下全部搜索结果。
- 已在后端实现 Tavily settings 读取、`tavily_search` builtin tool、`web_research` profile，以及 settings API 返回的 Tavily 分组元信息。
- 已在 admin 设置页增加通用 `select` 配置项支持，用于 Tavily 启用开关、默认 search depth 和 topic。
- 已补充 Tavily tool、settings API 和 `web_research` profile 的基础测试。

## 考虑过的替代方案

- 走 MCP server：
  灵活，但增加部署和发现成本，不符合“内置能力”的目标。
- 走 skill HTTP 工具：
  实现快，但更适合用户级技能，不适合作为平台默认能力。
- 只加 builtin tool、不做 subagent profile：
  改动更小，但不能满足“内置 subAgent”这一使用场景。

## 影响

本次实现已经落到 `agent/internal/engine`、`agent/internal/subagent`、`agent/internal/storage`、`agent/internal/api` 和 admin 设置页。该决策也为未来引入更多内置联网工具或研究型 subagent profile 提供了统一接入模式。
