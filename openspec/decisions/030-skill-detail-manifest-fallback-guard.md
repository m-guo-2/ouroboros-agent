# SkillDetail 缺失 manifest 的兜底防崩

- **日期**：2026-03-02
- **类型**：代码变更
- **状态**：已实施

## 背景

技能详情页在渲染 `SkillDetail` 时，默认假设接口返回的 `skill.manifest` 一定存在。
当返回数据出现不完整（例如 `manifest` 缺失）时，页面会在读取 `m.name` 时报错，触发 ErrorBoundary，导致页面不可用。

## 决策

在前端详情页增加 `manifest` 的防御性兜底：当 `skill.manifest` 缺失时构造一个最小可用的 fallback manifest，保证页面可渲染、可查看并可继续编辑。

## 变更内容

- 修改 `admin/src/components/features/skills/skill-detail.tsx`：
  - 新增 `createFallbackManifest(skillName)`，提供默认字段（`name/description/version/type/enabled/triggers/tools`）。
  - 在 `syncFromSkill` 中改为优先使用 `skill.manifest`，缺失时回退到 fallback，避免进入编辑态时因空值崩溃。
  - 在渲染阶段同样使用 fallback，避免读取 `m.name`、`m.enabled` 等字段时报错。

## 考虑过的替代方案

- 仅在渲染处做 `if (!skill.manifest) return 错误态`：
  - 可以避免崩溃，但会让用户无法继续查看/修复该技能配置。
  - 最终未采用，因为可用性较差。

## 影响

- 前端对后端短暂不一致或脏数据更具容错能力，减少页面白屏/崩溃。
- 不改变后端数据契约；后续仍建议排查为何出现缺失 `manifest` 的响应，作为根因治理。
