# cardrender — 卡片渲染引擎

将 HTML 内容渲染为 PNG 图片并上传到 OSS，返回可访问的 presigned URL。

## 架构

```
调用方（render_card 工具 / data_report 子 Agent）
    │
    ▼
cardrender.RenderCard(ctx, html, opts)
    │
    ├── 1. 输入校验（HTML 非空）
    ├── 2. 模板填充（如指定 template + data）
    ├── 3. Rod headless Chrome 渲染 → PNG 截图
    ├── 4. 截图校验（尺寸非零）
    ├── 5. OSS 上传 → PresignGetURL（7 天有效）
    └── 6. 返回 RenderResult{ImageURL, OSSKey, Width, Height}
```

## 核心类型

```go
type RenderOptions struct {
    Width   int           // viewport 宽度，默认 600
    Timeout time.Duration // 渲染超时，默认 30s
}

type RenderResult struct {
    ImageURL string // OSS presigned URL
    OSSKey   string // 对象存储 key
    Width    int    // 图片宽度 px
    Height   int    // 图片高度 px
}
```

## 使用方式

### 直接渲染 HTML

```go
result, err := cardrender.RenderCard(ctx, "<html><body><h1>Hello</h1></body></html>", cardrender.RenderOptions{
    Width: 600,
})
// result.ImageURL → "https://oss.example.com/cards/xxx.png?token=..."
```

### 模板模式

```go
html, err := cardrender.RenderTemplate("kpi", map[string]any{
    "title": "月活跃用户",
    "value": "128,000",
    "trend": "+12.5%",
    "trendUp": true,
    "comparison": "较上月",
})
if err != nil { ... }

result, err := cardrender.RenderCard(ctx, html, cardrender.RenderOptions{})
```

## 模板列表

| 模板名 | 用途 | 数据字段 |
|--------|------|----------|
| `kpi` | KPI 大数字展示 | title, value, trend, trendUp, comparison |
| `table` | 数据对比表格 | title, headers, rows |
| `status-board` | 状态面板 | title, items[{name, status}] |
| `ranking` | 排行榜 | title, items[{name, value}] |
| `timeline` | 事件时间线 | title, events[{time, description}] |
| `summary` | 信息摘要 | title, items[{label, value}] |

模板文件位于 `agent/data/card-templates/`，使用 Go `text/template` 语法。

## Browser 管理

- 单例 Chrome 实例，跨请求复用
- 空闲 5 分钟自动回收
- 最大并发渲染数：3（超出排队）
- 依赖环境中的 Chromium（Docker 镜像内置，开发环境 Rod 自动下载）

## 错误分类

| 错误类型 | 含义 |
|----------|------|
| `ErrInvalidInput` | HTML 为空或模板名不存在 |
| `ErrRenderTimeout` | 渲染超时 |
| `ErrBrowserUnavailable` | Chromium 不可用或启动失败 |
| `ErrOSSUploadFailed` | OSS 上传或签名失败 |

## 依赖

- [go-rod/rod](https://github.com/go-rod/rod) — headless Chrome
- `shared/oss` — 对象存储
- 环境 Chromium 二进制
