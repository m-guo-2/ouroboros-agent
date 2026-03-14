## Context

当前 Agent 系统已有三种子 Agent Profile（developer、file_analysis、web_research），通过 `subagent.Manager` 异步调度，每种 Profile 定义独立的工具集和 system prompt。消息发送统一由主 Agent 通过 `send_channel_message` 工具完成，子 Agent 不直接面向用户。

现有 channel 层已支持 image 消息类型（飞书通过 image_key 上传、企微通过 imgUrl），OSS 层（`shared/oss`）提供 PutObject 和 PresignGetURL 接口。

本次需要在现有子 Agent 框架上新增 `data_report` Profile，赋予 Agent "出图"能力：把复杂信息渲染为可视化卡片图片发送给用户。

**约束（来自用户决策）：**
- 部署环境需解决 Chromium 安装问题（Docker 镜像内置）
- 主 Agent 统一控制消息发送，子 Agent 只管出图 + 返回 URL
- 不做渠道适配，统一生成图片发送
- 预置 HTML 卡片模板
- 何时触发由 Agent 自行判断

## Goals / Non-Goals

**Goals:**
- 主 Agent 可将结构化数据委托给 data_report 子 Agent，生成可视化卡片 PNG
- 子 Agent 返回 OSS 图片 URL，主 Agent 通过 image 消息发送给用户
- 预置常见卡片模板，子 Agent 可选用模板或自由生成 HTML
- 渲染失败时 fallback 为格式化文本，保证功能降级
- Rod 集成进 agent 二进制，无需额外渲染服务进程

**Non-Goals:**
- 不做实时交互式卡片（如飞书 interactive card）
- 不做渠道差异化适配，统一走图片
- 不做复杂图表库集成（ECharts 等），卡片以信息排版为主，复杂图表可后续迭代
- 不修改子 Agent 现有的"不直接发消息"约定

## Decisions

### D1: 渲染引擎选 Rod（Go headless Chrome）

**选择**: [go-rod/rod](https://github.com/go-rod/rod) — 纯 Go 的 Chrome DevTools Protocol 客户端。

**备选方案**:
| 方案 | 优点 | 缺点 |
|------|------|------|
| Rod (Go) | 纯 Go、无额外进程、API 简洁、支持截图 | 需要 Chromium 二进制 |
| Puppeteer (Node) | 生态成熟 | 需要 Node runtime，多一个进程 |
| wkhtmltoimage | 轻量 | CSS 支持差，不支持 flexbox/grid |
| go-echarts | 纯 Go | 只能画图表，做不了自由排版 |

**理由**: Rod 直接集成进 agent Go 二进制，不引入额外运行时。Chromium 在 Docker 中通过 `chromium-browser` 包安装即可。对于低频报表生成场景（非实时渲染），启动开销可接受。Rod 内置 browser 下载管理，开发环境也无需手动安装。

### D2: 新增 `cardrender` 包封装渲染链路

**选择**: 在 `agent/internal/cardrender/` 下新建包，封装完整链路：

```
HTML string → Rod 渲染 → PNG 截图 → OSS 上传 → 返回 presign URL
```

包暴露一个核心函数：

```go
type RenderResult struct {
    ImageURL string
    OSSKey   string
    Width    int
    Height   int
}

func RenderCard(ctx context.Context, html string, opts RenderOptions) (*RenderResult, error)
```

**理由**: 把浏览器管理、截图、上传三个关注点聚合在一个包内，调用方（子 Agent 工具）只需传 HTML 拿 URL，不关心底层实现。

### D3: 模板机制 — 预置模板 + 自由生成双轨

**选择**: 在 `agent/data/card-templates/` 下预置 HTML 模板文件，每个模板是一个独立 HTML 文件，使用 Go `text/template` 占位符。子 Agent 有两种使用方式：

1. **模板模式**: 调用 `render_card` 时指定 `template` 名称 + `data` JSON，工具自动填充模板
2. **自由模式**: 直接传入完整 `html` 字符串，工具原样渲染

预置模板类型：
- `kpi` — 单/多 KPI 大数字展示
- `table` — 数据对比表格
- `status-board` — 多项目状态面板（绿黄红）
- `ranking` — 排行榜/Top N
- `timeline` — 事件时间线
- `summary` — 通用信息摘要卡片

**理由**: 模板方案保证基础质量稳定（不依赖 LLM 每次都写出合格的 HTML），自由模式保留灵活性。初期先用模板兜底，观察 LLM 自由生成的质量后再调整比例。

### D4: 子 Agent Profile 设计

**选择**: 新增 `data_report` Profile，工具集为：

| 工具 | 用途 |
|------|------|
| `render_card` | 核心工具：渲染 HTML 为 PNG 并上传 |
| `read_file` | 读取数据文件（如主 Agent 写入的临时数据文件） |
| `list_dir` | 列出模板目录 |

不给 `shell`、`write_file`——子 Agent 不需要执行任意命令或写文件，只需要读数据 + 调渲染。

**System Prompt 要点**:
- 你是 data_report 子代理，专职将数据转化为可视化卡片
- 分析数据特征，选择最合适的卡片类型
- 优先使用预置模板，数据结构不匹配时再自由生成 HTML
- 输出必须包含 imageUrl 字段
- 渲染失败时，返回格式化文本作为 fallback

### D5: 主 Agent 触发策略 — prompt 引导自主判断

**选择**: 在主 Agent 的 system prompt 中增加引导段，描述何时应调用 data_report 子 Agent，但不做硬编码触发规则。

引导示例：
- 当回复包含 3 个以上数值指标时
- 当需要对比多个维度的数据时
- 当汇总状态/进度信息时
- 当用户明确要求"出个图"/"做个报表"时

**理由**: 硬编码触发条件会很脆弱。LLM 本身擅长判断"什么时候用图比用字好"，只需给它足够的上下文和权限即可。

### D6: Fallback 策略

**选择**: 三级 fallback：

1. **模板渲染成功** → 返回图片 URL
2. **模板渲染失败，自由 HTML 渲染成功** → 返回图片 URL
3. **渲染完全失败**（Chromium 不可用、OSS 不可达等）→ 子 Agent 在结果中标记 `fallback: true`，附带格式化纯文本版本；主 Agent 改用 text/rich_text 发送

**理由**: 渲染链路依赖外部组件（Chromium、OSS），需要有降级路径。用户宁可看到一段格式化文字，也不应该看到错误消息。

## Risks / Trade-offs

- **[Chromium 资源占用]** → Rod 启动 Chromium 有内存和启动时间开销。Mitigation：实现 browser 实例复用池（单例或限数量），空闲超时后关闭。初期可用单实例 + 超时回收。
- **[LLM 生成 HTML 质量不稳定]** → 自由模式下 LLM 可能生成有 bug 的 HTML。Mitigation：预置模板作为主要路径；自由模式生成的 HTML 做基本校验（非空、有 body 标签）；截图后校验图片尺寸非零。
- **[OSS presign URL 过期]** → 图片 URL 有有效期，过期后用户无法查看历史消息中的图片。Mitigation：设置较长过期时间（7 天），或考虑使用公开读的 bucket/路径。
- **[Docker 镜像体积增大]** → Chromium 约增加 200-300MB。Mitigation：使用 `chromium-browser` 最小安装包；如果体积敏感，可拆分为独立的渲染 sidecar 服务（后续优化）。
- **[并发渲染]** → 多个 session 同时触发渲染可能导致资源争抢。Mitigation：browser 实例池限制并发数，超出排队或快速失败走 fallback。
