## Why

主 Agent 输出给用户的信息只有纯文字一种形态。当信息密度高（多维数据对比、趋势汇总、状态概览等），纯文字在 IM 场景下体验很差：关键数字淹没在段落里，长文本被折叠，用户没有耐心也看不明白。需要一个机制让 Agent 能把复杂信息转为可视化卡片图片，直接发给用户。

## What Changes

- 新增 `data_report` 子 Agent Profile，专职将结构化数据/复杂信息转换为 PNG 卡片图片
- 新增 `render_card` 内置工具，封装 HTML → headless Chrome 截图 → OSS 上传 → 返回图片 URL 的完整链路
- 在 agent 中集成 Rod（Go headless Chrome 库）作为渲染引擎
- 预置一组 HTML 卡片模板（KPI 大数字、多指标仪表盘、对比表格、排行榜、状态面板等），子 Agent 可选用或自由生成
- 子 Agent 完成后返回图片 URL 给主 Agent，由主 Agent 通过 `send_channel_message(messageType: "image")` 统一发送
- 渲染失败时 fallback 为格式化纯文本，保证功能降级而非完全失败
- 主 Agent system prompt 中增加引导，让其自行判断何时调用 data_report 子 Agent

## Capabilities

### New Capabilities
- `card-rendering`: HTML 卡片渲染引擎——接收 HTML 内容，通过 headless Chrome 截图生成 PNG，上传到 OSS 并返回可访问 URL
- `data-report-subagent`: data_report 子 Agent Profile 定义——prompt、可用工具集、卡片模板、fallback 策略

### Modified Capabilities
- `shared-oss-storage`: 渲染出的 PNG 图片需通过 OSS 上传并生成可访问 URL，需确认现有 OSS 接口满足需求（PutObject + PresignGetURL）

## Impact

- **新依赖**: `github.com/go-rod/rod`（Go headless Chrome 库），部署环境需安装 Chromium
- **代码变更**: `agent/internal/subagent/` 新增 profile；`agent/internal/runner/processor.go` 注册新工具；新增 `agent/internal/cardrender/` 包
- **资源模板**: `agent/data/card-templates/` 下新增 HTML 模板文件
- **OSS**: 复用现有 `shared/oss` 包，需确认 presign URL 过期策略适合图片场景（建议 7 天或更长）
- **部署**: Docker 镜像需包含 Chromium；CI 可能需要调整
