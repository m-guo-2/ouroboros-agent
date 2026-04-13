## Context

当前系统通过单一 GitHub 仓库（`config.github.skills_repo`）管理所有 skill。`github.Client` 封装了 GitHub Contents API 的 CRUD 操作，`github.Store` 负责同步和缓存。Admin 前端提供 skill 的列表、详情、新建、同步管理。

用户希望能从外部公开 GitHub 仓库中浏览并挑选 skill 导入到自己的仓库，而不是手动复制文件。

## Goals / Non-Goals

**Goals:**
- 支持用户粘贴公开 GitHub 仓库 URL，浏览该路径下可用的 skill 列表
- 支持从浏览结果中选择若干 skill，批量导入到本地 skills 仓库
- 导入时检测 ID 冲突，防止意外覆盖
- Admin UI 提供完整的"粘贴 URL → 浏览 → 勾选 → 导入"交互流程

**Non-Goals:**
- 不支持私有仓库（需要额外 token 管理，可后续扩展）
- 不做来源追踪和上游更新检测（一次性复制，导入后即为本地 skill）
- 不做 Skill Marketplace / Registry 浏览（不维护 skill 源列表）
- 不处理 skill 内部的 scripts 依赖解析或兼容性校验

## Decisions

### 1. 复用现有 `github.Client`，为外部 repo 创建临时实例

**选择**：新增 `NewPublicClient(owner, repo, branch)` 工厂方法，创建不带 token 的 Client 实例用于读取公开仓库。

**备选**：复用系统 token 访问外部 repo。
**理由**：目标明确是公开仓库，无 token 的请求更简单安全，不会泄露系统 token 的权限范围。GitHub 对未认证请求有 60 次/小时的速率限制，单次浏览+导入操作远远够用。

### 2. URL 解析为结构化参数

**选择**：后端实现 `ParseGitHubURL(url) → (owner, repo, branch, path)` 函数，支持以下格式：
- `https://github.com/owner/repo` → path 默认为根目录
- `https://github.com/owner/repo/tree/branch/path/to/skills` → 提取 branch 和 path
- `owner/repo` 简写 → branch 默认 main，path 默认根目录

**理由**：用户从浏览器复制的 URL 格式多样，需要统一处理。解析逻辑集中在后端，前端只管传原始 URL。

### 3. 两阶段 API：browse + import

**选择**：分两个端点，browse 返回列表，import 执行写入。

- `POST /api/skills/import/browse` — 列出目标路径下的 skill（读取每个子目录的 SKILL.md frontmatter）
- `POST /api/skills/import` — 将选中的 skill 文件写入本地 repo

**备选**：单一端点一步完成。
**理由**：两阶段让用户看到可选列表后再决定导入哪些，避免盲目全量导入。browse 是只读操作，import 是写操作，职责清晰。

### 4. 导入写入目标：commit 到用户的 skills GitHub 仓库

**选择**：导入操作通过主 `Store` 的 `client.PutFile()` 将文件直接 commit 到用户配置的 `skills_repo` GitHub 仓库，和现有 `Create`/`Update` 流程一致。导入后触发 `refresh()` 同步本地缓存。

**备选**：只写本地磁盘/SQLite，不提交到 GitHub。
**理由**：skill 仓库是唯一的持久化 source of truth。只写本地会在进程重启或重新 sync 时丢失。commit 到 GitHub 保证导入的 skill 与手动创建的 skill 生命周期完全一致——持久、可版本回溯、可在其他实例同步。

### 5. 导入粒度：整个 skill 目录

**选择**：以 skill 目录为单位导入，复制 `SKILL.md` + `scripts/` + `references/` 完整目录结构。每个文件单独一次 `PutFile` commit。

**理由**：这和现有 `syncSkillToDisk` / `writeFiles` 的粒度一致。skill 是最小可用单元，拆分文件级导入没有实际价值。

### 6. ID 冲突处理

**选择**：browse 返回时标记每个 skill 是否已存在于本地（`exists: true`）。import 时如果 skill ID 已存在，默认拒绝，需要用户显式传 `overwrite: true` 才覆盖。

**理由**：安全优先，防止误操作覆盖已定制的本地 skill。

### 7. 前端导入对话框交互设计

导入对话框是用户完成"浏览 → 挑选 → 导入"全流程的唯一入口，需要在一个对话框内处理好三个阶段的状态切换和反馈。

**交互流程：**

1. **输入阶段** — 对话框顶部是 URL 输入框 + "浏览"按钮。输入框下方有一行 helper text 说明支持的 URL 格式（完整 GitHub URL 或 `owner/repo` 简写），降低用户认知成本。
2. **加载阶段** — 点击"浏览"后，列表区域展示骨架屏（3-4 行 Skeleton），和项目其他加载态保持一致。GitHub 返回 403 时需要区分展示为"速率限制"而非通用错误。
3. **选择阶段** — 浏览成功后：
   - 列表上方展示仓库上下文（`owner/repo` + path），让用户确认来源
   - 每个 skill 行使用 lucide 图标（`Square` / `CheckSquare`）替代原生 checkbox，与项目 icon 体系一致
   - 已存在本地的 skill 置灰不可选，右侧用 Badge 标记"已存在"
   - 支持全选/取消全选
4. **导入阶段** — 点击"导入"后按钮变为 loading 态，展示"正在导入 N 个技能…"文案。因为每个 skill 导入涉及多次 GitHub API 调用（读源 + 写目标），可能需要 5-20 秒。
5. **结果阶段** — 导入完成后展示成功/失败摘要（绿色/琥珀色卡片），成功的 skill 在列表中自动标记为"已存在"。用户可以继续浏览其他 URL 或关闭对话框。

**视觉规范：**
- 对话框宽度 `max-w-2xl`，给列表更充裕的空间
- 列表区域用 `ScrollArea` 限制最大高度，避免对话框撑得过长
- 选中行高亮用 `bg-blue-50`，和项目的蓝色 accent 一致
- 所有 loading 态使用 `Loader2` 旋转图标，和项目其他按钮的 loading 风格统一

## Risks / Trade-offs

- **GitHub API 速率限制** → 未认证请求 60 次/小时。单次 browse 需要 1（ListDir）+ N（读 SKILL.md frontmatter）次请求。如果目标 repo 有大量 skill，可能接近限制。缓解：browse 时只读 frontmatter，不下载完整内容；如需扩展，可 fallback 到带 token 请求。
- **大文件/大目录** → 如果 skill 的 scripts 或 references 文件很大，GitHub Contents API 有 1MB 文件大小限制。缓解：对于 MVP 阶段，接受此限制；超大文件场景极少出现。
- **导入后 skill 与上游脱节** → 一次性复制，上游更新后本地不会自动同步。这是有意的 non-goal，但用户可能不知道。缓解：导入成功后的提示中明确说明"已导入为本地 skill"。
