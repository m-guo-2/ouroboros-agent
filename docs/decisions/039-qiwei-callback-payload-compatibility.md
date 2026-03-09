# 企微回调载荷兼容解析与 ACK 策略修正

- **日期**：2026-03-04
- **类型**：代码变更
- **状态**：已实施

## 背景

`channel-qiwei` 在回调入口使用严格结构体解码（`code/msg/data[]`）。联调时发现部分回调（尤其是“验证回调地址是否可用”）并不稳定符合该结构，导致入口直接返回 `400 invalid callback body`。

根据 QiWe 文档约束，回调接口应在 3 秒内响应，否则平台可能重试；因此解析失败直接 `400` 会放大重试噪音并干扰联调。

## 决策

将回调解析改为“标准优先 + 兼容兜底”：

1. 优先按标准结构 `code/msg/data[]` 解析。
2. 兼容 `data` 为对象、数组、字符串及单消息直推形态。
3. 识别“验证回调地址是否可用”类 payload，视为有效探活回调。
4. 解析失败时记录原始 body（截断）并返回 `200` ACK，避免重试风暴。

## 变更内容

- 修改 `channel-qiwei/events.go`
  - `handleWebhookCallback` 改为先读取 `rawBody`，使用 `parseCallbackMessages` 解析。
  - 新增兼容函数：
    - `parseCallbackMessages`
    - `decodeMessageArray`
    - `decodeOneMessage`
    - `truncateBody`
  - 解析失败日志包含错误与截断后的原始回调内容。
  - 保持“快速 ACK + 异步处理”主流程不变。

## 影响

- 回调入口对 QiWe 文档描述的验证回调和非标准形态更稳健。
- 减少 `invalid callback body` 引发的平台重试，联调可观测性更好。
- 未改变消息归一化和 Agent 转发语义，仅增强入口容错。
