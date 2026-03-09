## Why

当前 subagent 体系只能处理文件和 Shell，缺少一个“可操作真实网页”的受限执行面。浏览器场景里真正困难的不是“会不会点按钮”，而是如何稳定理解页面、在扫码/验证码/MFA 等人工关卡前优雅暂停，并在用户协助后继续执行。

参考 OpenClaw 的经验后，这个变更需要从“给模型一堆 selector 工具”调整为“给模型一个稳定的页面交互模型”。V1 的重点不是做成完整浏览器平台，而是先把 `browser` subagent 的运行时、页面理解方式和人工 checkpoint 控制流做对。

## What Changes

- 新增 `browser` subagent profile，运行在隔离的浏览器沙箱中
- 浏览器工具从 selector 导向改为 `browser_navigate`、`browser_snapshot`、`browser_act`、`browser_screenshot`
- `browser_snapshot` 返回面向 LLM 的页面结构摘要和可交互元素 `ref`，后续动作通过 `browser_act(ref, action)` 完成，而不是依赖 CSS selector
- 新增 `request_human_intervention` 工具，用于扫码登录、验证码、MFA 等人工关卡
- 人工介入采用轻量 checkpoint 流程：通知用户、暂停等待、用户回复后恢复执行
- 远程接管浏览器（如 noVNC / VNC）不作为 V1 必需能力，只作为后续增强方向

## Capabilities

### New Capabilities
- `browser-sandbox-tools`: 浏览器子代理运行时与页面交互模型，包含 navigate、snapshot、act、screenshot 等能力
- `human-intervention-flow`: 人工 checkpoint 机制，包含通知、等待、恢复、超时与回复拦截

### Modified Capabilities

（无已有 spec 变更）

## Impact

- **agent/internal/subagent/**: 扩展 `browser` profile 和对应 system prompt
- **agent/internal/engine/ostools/**: 新增浏览器工具与人工介入工具
- **agent/internal/runner/**: 将用户回复优先路由到 pending checkpoint，而不是总是进入主 agent loop
- **浏览器运行时**: 需要引入 headless browser 驱动与页面快照/ref 映射逻辑
- **渠道层**: 继续复用 `channels.SendToChannel`，不新增新的消息基础设施
