# 模型发现固定走官方 Provider API

- **日期**：2026-03-04
- **类型**：代码变更
- **状态**：已实施

## 背景

模型配置中的 `base_url` 允许为运行时推理设置兼容路径（如 Moonshot 的 `/anthropic`）。  
但“查询可用模型列表”属于管理侧发现能力，不应复用运行时兼容地址，否则会出现 `404` 或返回结构不一致，最终在 UI 侧表现为“模型列表为空”。

## 决策

将模型发现接口与运行时调用解耦：`/api/models/:id/available-models` 固定调用各家官方模型列表 API，忽略配置中的 `base_url`。

## 变更内容

- 修改 `agent/internal/api/models_fetch.go`：
  - `fetchAvailableModels` 对 `baseURL` 显式忽略，仅按 `provider` 分发。
  - Claude 固定调用 `https://api.anthropic.com/v1/models?limit=100`。
  - OpenAI 固定调用 `https://api.openai.com/v1/models`。
  - Kimi 固定调用 `https://api.moonshot.cn/v1/models`。
  - GLM 固定调用 `https://open.bigmodel.cn/api/paas/v4/models`。
  - provider 不支持时返回明确错误，避免静默空结果。
- 新增 `GET /api/settings/provider-models?provider=...`（`agent/internal/api/settings.go` + `router.go`）：
  - Agent 详情页“查询模型”按钮改由该接口提供数据。
  - 从 `settings` 读取 provider 对应 API Key，再复用统一的官方端点发现逻辑。
  - 修复原先该路径被误当作普通 settings key 读取、导致前端拿不到模型列表的问题。

## 考虑过的替代方案

- 方案 A：继续复用 `base_url` 进行模型发现。  
  否决原因：`base_url` 语义是“运行时推理入口”，可能是兼容层地址，不保证支持模型枚举；会导致发现链路不稳定。

## 影响

- 运行时模型调用兼容能力保持不变（仍可使用 `/anthropic` 等兼容地址）。
- 管理端“获取模型列表”结果更可预测，降低因兼容路径导致的空列表问题。
- 若凭证错误，返回将更贴近真实 Provider 响应，便于排查配置问题。
