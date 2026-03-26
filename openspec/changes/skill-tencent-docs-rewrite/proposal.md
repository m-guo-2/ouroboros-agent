## Why

tencent-docs skill 的 SKILL.md 当前是 MCP API 参考文档的风格，直接暴露了 `tencent_docs_api({ name, arguments })` 的调用方式。但主 agent 上下文中并不存在 `tencent_docs_api` 工具——该能力由 MCP server 提供，模型无法直接调用。

实际触发的问题（trace-b7bbf990596b0de2）：
1. 模型 `load_skill("tencent-docs")` 获得了 README
2. README 指引它调 `tencent_docs_api` → Tool not found
3. 模型自行恢复走 `run_subagent_async`，浪费 28 秒 + 1425 tokens

根因：
- skill 没有 scripts，`scripts/` 目录为空
- SKILL.md 用 MCP 开发者文档的视角写，而不是产品使用文档的视角

## Design Principles

**给 agent 的 skill 描述应该像产品文档，不是 API 文档。**

1. **简单需求一步完成**：一个 script 调用解决一件事（创建文档、读取内容、搜索文件）
2. **不暴露内部概念**：file_id、sheet_id、MCP 调用方式、轮询 progress 等细节封装在 script 内部
3. **API 组合是高阶玩法**：只有复杂需求（编辑已有文档的特定元素、批量操作表格字段）才需要参考 references 中的详细 API 文档
4. **参数尽量语义化**：传标题和内容，不传 file_id 和 JSON schema

## What Changes

### 1. 新增 scripts

| 脚本 | 用途 | 核心参数 | 返回 |
|------|------|---------|------|
| `create_doc` | 创建在线文档 | `--title` `--markdown` `[--type smartcanvas\|word]` | 文档 URL |
| `create_sheet` | 创建智能表格 | `--title` `--fields JSON` `--records JSON` | 表格 URL |
| `create_excel` | 创建在线表格 | `--title` `--markdown` | 表格 URL |
| `create_slide` | 创建幻灯片 | `--description` | 任务 ID（异步，需轮询） |
| `create_mind` | 创建思维导图 | `--title` `--markdown` | 文档 URL |
| `create_flowchart` | 创建流程图 | `--title` `--mermaid`（纯英文） | 文档 URL |
| `read_doc` | 读取文档内容 | `--url` 或 `--file_id` | Markdown 内容 |
| `search_file` | 搜索文件 | `--query` `[--type title\|owner]` | 文件列表 |
| `list_recent` | 最近文档 | `[--count N]` | 文件列表 |

每个脚本内部封装：
- 环境变量读取 TENCENT_DOCS_TOKEN
- MCP server 调用（HTTP）
- 异步操作的轮询等待（如 PPT 生成）
- 错误处理和友好提示

### 2. 重写 SKILL.md

从 MCP API 文档 → 产品能力文档。见 `SKILL.md.draft` 文件。

### 3. references 保留

原有的 API 参考文档（`api_references.md`、`smartsheet_references.md` 等）保留在 `references/` 目录，供模型在复杂需求时通过 `load_skill_reference` 按需加载。
