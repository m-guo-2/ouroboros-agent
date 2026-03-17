## Why

子 agent（subagent）当前强制继承主 agent 的 provider 和 model，无法独立选择模型。在实际场景中，主 agent 需要强推理能力（如 Claude Sonnet/Opus），而子 agent 的 `web_research`、`file_analysis` 等轻量 profile 用更快、更便宜的模型即可满足需求。缺少这一配置能力导致不必要的 token 消耗和延迟。

## What Changes

- 新增 subagent profile 级别的模型配置：允许管理员为每个 subagent profile（developer、file_analysis、web_research、data_report）单独指定 provider + model，也可以留空以继承主 agent 配置。
- Admin UI 新增子 agent 模型配置面板：在 agent 详情页新增 tab 或 section，管理员可为各 profile 配置独立模型。
- Runner/Processor 修改：启动子 agent 时，优先使用 profile 级别的模型配置，若未配置则回退到主 agent 的模型。
- API 扩展：agent 的 CRUD API 支持读写 subagent model 配置。

## Capabilities

### New Capabilities
- `subagent-model-override`: 为 subagent profile 提供独立的 provider + model 配置能力，包括存储、API、admin UI、运行时解析。

### Modified Capabilities

## Impact

- **存储层**：`agent_configs` 表新增 `subagent_models` 字段（JSON 格式），或新建独立表。
- **API**：`PUT /api/agents/{id}` 请求体新增 subagent model 配置字段；`GET /api/agents/{id}` 响应体同步返回。
- **Runner**：`processor.go` 中子 agent 启动逻辑需读取 profile 对应的模型配置并构建独立 LLM client。
- **Admin UI**：`agent-detail.tsx` 新增子 agent 模型配置 UI。
- **无破坏性变更**：所有 profile 的 model 配置默认为空，回退到主 agent 模型，保持向后兼容。
