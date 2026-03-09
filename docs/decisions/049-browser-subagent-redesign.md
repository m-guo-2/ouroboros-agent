# Browser Subagent 重设计

- **日期**：2026-03-06
- **类型**：架构决策
- **状态**：已决定

## 背景

最初的 browser subagent 方案沿用了文件工具的思路，把浏览器能力拆成若干基于 CSS selector 的 click/type/read 工具。这种设计实现简单，但在真实网页里不稳定，也没有把扫码、验证码、MFA 这类人工关卡作为一等控制流处理。

## 决策

browser subagent 改为采用 `snapshot -> ref -> act` 的页面交互模型，并将人工介入设计为内建 checkpoint 能力。V1 只做轻量通知与恢复，不把远程桌面接管作为前置能力。

## 变更内容

- 重写 `openspec/changes/browser-sandbox-subagent/proposal.md`
- 重写 `openspec/changes/browser-sandbox-subagent/design.md`
- 重写 `openspec/changes/browser-sandbox-subagent/specs/browser-sandbox-tools/spec.md`
- 重写 `openspec/changes/browser-sandbox-subagent/specs/human-intervention-flow/spec.md`
- 重写 `openspec/changes/browser-sandbox-subagent/tasks.md`
- 将浏览器工具面收敛为 `browser_navigate`、`browser_snapshot`、`browser_act`、`browser_screenshot`
- 将人工介入定义为 checkpoint 流程：通知、等待、用户回复恢复、超时清理

## 考虑过的替代方案

继续沿用 selector 工具集。该方案开发更快，但稳定性和可调试性较差。

直接引入 OpenClaw 式的 VNC/noVNC 远程接管。该方案能力更强，但会显著扩大安全面与系统复杂度，不适合作为 V1 前提。

## 影响

后续实现将优先围绕“稳定页面理解”和“控制流正确性”展开，而不是追求浏览器能力数量。未来如果人工关卡成为高频瓶颈，可以在当前 checkpoint 机制上继续叠加远程接管能力。
