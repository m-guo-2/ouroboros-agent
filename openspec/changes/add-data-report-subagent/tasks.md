## 1. cardrender 包 — 渲染引擎核心

- [ ] 1.1 创建 `agent/internal/cardrender/` 包，定义 `RenderResult` 结构体和 `RenderCard(ctx, html, opts) (*RenderResult, error)` 函数签名
- [ ] 1.2 集成 Rod：实现 HTML → headless Chrome 页面加载 → 全页 PNG 截图的核心渲染逻辑，支持自定义 viewport 宽度和渲染超时
- [ ] 1.3 实现 browser 实例管理：单例复用、空闲超时回收（默认 5 分钟）、并发数限制（默认 3）
- [ ] 1.4 集成 OSS 上传：截图完成后调用 `shared/oss` 的 PutObject 上传 PNG，再调 PresignGetURL 生成 7 天有效的 presigned URL
- [ ] 1.5 实现输入校验：空 HTML 拒绝、截图尺寸零值检测，返回分类错误（render_timeout / browser_unavailable / oss_upload_failed / invalid_template）

## 2. 模板系统

- [ ] 2.1 创建 `agent/data/card-templates/` 目录，定义模板加载机制（从嵌入文件或磁盘目录读取）
- [ ] 2.2 实现模板引擎：接收 template name + data JSON，用 Go `text/template` 填充数据后输出完整 HTML
- [ ] 2.3 编写 `kpi` 模板 — 大数字 + 趋势箭头 + 对比基线
- [ ] 2.4 编写 `table` 模板 — 表头高亮 + 交替行色的数据表格
- [ ] 2.5 编写 `status-board` 模板 — 绿黄红色块状态面板
- [ ] 2.6 编写 `ranking` 模板 — 横向条形排行榜
- [ ] 2.7 编写 `timeline` 模板 — 垂直时间线 + 事件节点
- [ ] 2.8 编写 `summary` 模板 — 通用 key-value 信息摘要卡片
- [ ] 2.9 模板未找到时返回错误并列出所有可用模板名

## 3. render_card 工具注册

- [ ] 3.1 在 `agent/internal/subagent/manager.go` 的 `allowedToolsForProfile` 中新增 `data_report` profile，允许 `render_card`、`read_file`、`list_dir`
- [ ] 3.2 在 `agent/internal/runner/processor.go` 中注册 `render_card` 工具，schema 包含 `template`（可选）、`data`（可选 JSON 对象）、`html`（可选字符串）、`width`（可选数字），执行时调用 cardrender 包
- [ ] 3.3 工具返回值包含 `imageUrl`、`width`、`height`；失败时返回包含错误分类的 error message

## 4. data_report 子 Agent Profile

- [ ] 4.1 在 `normalizeProfile` 和 `profileDisplayName` 中增加 `data_report` 分支
- [ ] 4.2 编写 `defaultPromptByProfile("data_report")` system prompt：角色定义、模板选择策略、输出格式要求（imageUrl / cardType / summary）、fallback 指令
- [ ] 4.3 实现 fallback 逻辑：system prompt 指导子 Agent 在 render_card 失败时生成格式化文本，结果中包含 `fallback: true` 和 `fallbackText`

## 5. 主 Agent 集成

- [ ] 5.1 在主 Agent system prompt（`agent/data/prompts/` 或 processor 中的 prompt 拼接处）追加 data_report 触发引导段
- [ ] 5.2 确保主 Agent 收到子 Agent 返回的 imageUrl 后，能正确调用 `send_channel_message(messageType: "image", content: imageUrl)` 发送
- [ ] 5.3 确保主 Agent 收到 fallback 结果时，改用 text/rich_text 发送 fallbackText

## 6. 依赖与部署

- [ ] 6.1 在 `agent/go.mod` 中添加 `github.com/go-rod/rod` 依赖
- [ ] 6.2 更新 Dockerfile：安装 `chromium-browser` 包及必要字体（中文字体）
- [ ] 6.3 确认 OSS PresignGetURL 在当前配置下能生成 7 天有效期的 URL，必要时调整默认过期策略

## 7. 测试与验收

- [ ] 7.1 cardrender 包单元测试：mock browser 验证渲染流程、mock OSS 验证上传流程、错误分类测试
- [ ] 7.2 模板渲染测试：每个预置模板的 happy path 测试（给定数据 → 渲染成功 → PNG 非空）
- [ ] 7.3 子 Agent 集成测试：通过 `run_subagent_async(profile: "data_report")` 端到端验证完整链路
- [ ] 7.4 Fallback 测试：模拟 Chromium 不可用场景，验证子 Agent 返回格式化文本
