# 企微回调异步处理 context canceled 修复

- **日期**：2026-03-04
- **类型**：Bug 修复
- **状态**：已实施

## 背景

`channel-qiwei` 回调入口在返回 ACK 后异步处理消息，但 goroutine 复用了 `r.Context()`。HTTP 请求结束后该 context 会被取消，导致后续转发 Agent 出现：

`Post "http://localhost:1997/api/channels/incoming": context canceled`

## 决策

异步消息处理不再依赖请求生命周期 context，改为每条消息独立创建带超时的后台 context。

## 变更内容

- 修改 `channel-qiwei/events.go`
  - 在 goroutine 内将 `a.handleCallbackMessage(r.Context(), msg)` 改为：
    - `context.WithTimeout(context.Background(), 20*time.Second)`
  - 增加 `msgType=2` 的文本提取分支，与映射保持一致。

## 影响

- 回调 ACK 后，异步转发不再被请求结束连带取消。
- `context canceled` 噪音显著减少，联调稳定性提升。
