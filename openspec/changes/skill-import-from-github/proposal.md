## Why

当前所有 skill 只能从一个配置好的 GitHub 仓库管理（手动在仓库中创建，或通过 admin 表单新建）。当用户发现外部公开仓库中有想用的 skill 时，需要手动复制文件、提交到自己的仓库——流程繁琐且容易出错。

需要一个"从外部 GitHub 仓库浏览并导入 skill"的便捷入口，让用户粘贴一个 GitHub 地址就能挑选并导入想要的 skill。

## What Changes

- 新增"浏览外部 skill 源"能力：给定一个公开 GitHub 仓库 URL（可以是 repo 根目录或子路径），列出该路径下所有包含 `SKILL.md` 的 skill 目录及其基本信息（name、description）
- 新增"导入外部 skill"能力：从浏览结果中选择若干 skill，将其 `SKILL.md`、`scripts/`、`references/` 完整复制到本地 skills 仓库
- 导入时检测 ID 冲突，已存在的 skill 标识提示
- Admin 前端新增"导入 Skill"对话框，支持粘贴 URL → 浏览 → 勾选 → 批量导入的完整流程

## Capabilities

### New Capabilities
- `skill-import`: 从外部公开 GitHub 仓库浏览和导入 skill 到本地 skills 仓库的完整能力，包括 URL 解析、远程仓库浏览、文件复制、冲突检测，以及 Admin UI 的导入交互流程

### Modified Capabilities

（无已有 spec 需要修改）

## Impact

- **后端**：`agent/internal/github/` 新增 URL 解析和跨 repo 读取逻辑；`agent/internal/api/` 新增 import 相关端点
- **前端**：`admin/src/components/features/skills/` 新增导入对话框组件和相关 API/hooks
- **依赖**：无新外部依赖，复用现有 GitHub Contents API client
- **已有功能**：不影响现有 skill CRUD 和 sync 流程
