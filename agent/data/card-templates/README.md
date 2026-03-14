# 卡片模板

data_report 子 Agent 使用的 HTML 卡片模板。每个模板是一个独立的 HTML 文件，使用 Go `text/template` 语法。

## 模板规范

- 每个模板必须是完整、自包含的 HTML（含 `<html>`、`<head>`、`<body>` 标签）
- 所有样式内联在 `<style>` 标签中，不依赖外部 CSS
- 不依赖外部 JS 或字体 CDN（渲染环境可能无网络）
- 设计宽度 600px，高度自适应
- 使用系统字体栈：`-apple-system, "Noto Sans SC", "Microsoft YaHei", sans-serif`

## 数据约定

### kpi

```json
{
  "title": "月活跃用户",
  "value": "128,000",
  "trend": "+12.5%",
  "trendUp": true,
  "comparison": "较上月"
}
```

### table

```json
{
  "title": "销售对比",
  "headers": ["产品", "Q1", "Q2", "变化"],
  "rows": [
    ["产品A", "120", "156", "+30%"],
    ["产品B", "89", "72", "-19%"]
  ]
}
```

### status-board

```json
{
  "title": "系统状态",
  "items": [
    {"name": "API 服务", "status": "ok"},
    {"name": "数据库", "status": "warning"},
    {"name": "缓存", "status": "error"}
  ]
}
```

status 取值：`ok` / `warning` / `error`

### ranking

```json
{
  "title": "销售排行",
  "items": [
    {"name": "张三", "value": 156},
    {"name": "李四", "value": 132}
  ]
}
```

### timeline

```json
{
  "title": "项目进展",
  "events": [
    {"time": "03-01", "description": "需求评审完成"},
    {"time": "03-05", "description": "开发启动"}
  ]
}
```

### summary

```json
{
  "title": "项目概况",
  "items": [
    {"label": "负责人", "value": "张三"},
    {"label": "状态", "value": "进行中"},
    {"label": "截止日期", "value": "2026-04-01"}
  ]
}
```
