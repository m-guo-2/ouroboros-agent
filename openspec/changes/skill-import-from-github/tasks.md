## 1. GitHub URL 解析与公开仓库 Client

- [x] 1.1 在 `agent/internal/github/` 新增 `ParseGitHubURL(url) → (owner, repo, branch, path, error)` 函数，支持完整 URL、tree/blob URL、shorthand 三种格式
- [x] 1.2 新增 `NewPublicClient(owner, repo, branch)` 工厂方法，创建不带 token 的 Client 实例
- [x] 1.3 为 URL 解析编写单元测试，覆盖各种 URL 格式和异常输入

## 2. Browse API — 浏览外部 skill 列表

- [x] 2.1 在 `agent/internal/github/` 新增 `BrowseSkills(client, basePath) → []BrowseSkillEntry` 函数，列出目标路径下所有包含 SKILL.md 的子目录，读取 frontmatter 返回 name/description
- [x] 2.2 在 `agent/internal/api/skills.go` 新增 `POST /api/skills/import/browse` 端点，接收 `{ url }` 参数，调用 URL 解析 + BrowseSkills，对比本地 skill 标记 `exists` 后返回
- [x] 2.3 在 `agent/internal/api/router.go` 注册新路由

## 3. Import API — 导入选中的 skill

- [x] 3.1 在 `agent/internal/github/` 新增 `ImportSkill(srcClient, srcPath, dstStore, skillID, overwrite) error` 函数，从源 repo 读取 SKILL.md + scripts/ + references/ 并通过主 Store 的 `client.PutFile()` commit 到用户的 skills GitHub 仓库
- [x] 3.2 在 `agent/internal/api/skills.go` 新增 `POST /api/skills/import` 端点，接收 `{ repo, branch, path, skills[], overwrite }` 参数，批量调用 ImportSkill，完成后触发 refresh
- [x] 3.3 在 `agent/internal/api/router.go` 注册新路由
- [x] 3.4 处理 ID 冲突逻辑：无 overwrite 时已存在的 skill 返回冲突错误，有 overwrite 时覆盖写入

## 4. Admin 前端 — 导入对话框

- [x] 4.1 在 `admin/src/api/skills.ts` 新增 `browseImport(url)` 和 `importSkills(params)` API 方法
- [x] 4.2 在 `admin/src/hooks/use-skills.ts` 新增 `useBrowseImport` 和 `useImportSkills` hooks
- [x] 4.3 新增 `admin/src/components/features/skills/skill-import-dialog.tsx` 组件，实现两阶段交互：URL 输入 + 浏览 → 勾选 + 导入
- [x] 4.4 在 `skill-list.tsx` 的 header 区域新增"导入"按钮，触发导入对话框
- [x] 4.5 导入成功后自动刷新 skill 列表，显示导入结果（成功数量 + 失败原因）

## 5. 前端交互改进（Design #7）

- [x] 5.1 对话框加宽至 `max-w-2xl`，URL 输入框下方增加格式提示 helper text
- [x] 5.2 浏览加载时展示骨架屏（Skeleton），替代空白等待
- [x] 5.3 浏览成功后列表上方展示仓库上下文（`owner/repo` + path）
- [x] 5.4 用 lucide `Square`/`CheckSquare` 图标替代原生 checkbox
- [x] 5.5 导入中按钮文案改为"正在导入 N 个技能…"
