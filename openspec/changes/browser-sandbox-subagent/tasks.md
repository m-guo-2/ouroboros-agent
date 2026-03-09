## 1. Browser profile foundation

- [ ] 1.1 在 `agent/internal/subagent/manager.go` 中扩展 `browser` profile，并补齐 `normalizeProfile`、`profileDisplayName` 与 `allowedToolsForProfile`
- [ ] 1.2 为 `browser` profile 编写专用 system prompt，强调 snapshot-first、act-by-ref 和人工 checkpoint
- [ ] 1.3 为 browser subagent 增加独立并发限制，默认最大并发数为 2

## 2. Browser runtime

- [ ] 2.1 在 `agent/internal/engine/ostools/browser.go` 中建立浏览器运行时上下文（browser + single page）
- [ ] 2.2 在 subagent 生命周期中接入浏览器实例创建与清理
- [ ] 2.3 增加取消、超时场景下的浏览器清理保障

## 3. Snapshot-act interaction model

- [ ] 3.1 实现 `browser_navigate`，返回页面标题与最终 URL
- [ ] 3.2 实现 `browser_snapshot`，输出页面摘要、交互元素列表与临时 `ref`
- [ ] 3.3 为 `browser_snapshot` 设计最小稳定输出结构，包含 `role`、`name`、`text` 等字段
- [ ] 3.4 实现 `browser_act(ref, action, value?)`，至少支持 `click`、`type`、`clear`、`press`、`select`
- [ ] 3.5 实现 `ref` 失效时的明确错误与重新 snapshot 提示
- [ ] 3.6 实现 `browser_screenshot` 供调试与 checkpoint 使用

## 4. Human checkpoint manager

- [ ] 4.1 创建 checkpoint/intervention manager，支持 `Request`、`Resolve`、`PendingForSession`
- [ ] 4.2 在 `Request` 流程中接入超时等待和 pending 清理
- [ ] 4.3 在 subagent 取消时中断 waiting checkpoint 并清理状态

## 5. Checkpoint tool and routing

- [ ] 5.1 实现 `request_human_intervention` 工具
- [ ] 5.2 在工具中接入可选截图，并通过 `channels.SendToChannel` 发送通知
- [ ] 5.3 在 `agent/internal/runner/processor.go` 中增加 pending checkpoint 检查，优先消费用户回复以恢复 subagent
- [ ] 5.4 在 API 层增加 `POST /api/interventions/:id/resolve` 解除入口

## 6. Validation and hardening

- [ ] 6.1 编写 browser subagent 启动与清理测试
- [ ] 6.2 编写 snapshot -> act 的端到端测试
- [ ] 6.3 编写人工 checkpoint 的等待、用户回复恢复、API 恢复测试
- [ ] 6.4 验证 browser subagent 达到并发上限时返回预期错误
- [ ] 6.5 评估并记录 V1 URL 安全策略和后续远程接管增强路径
