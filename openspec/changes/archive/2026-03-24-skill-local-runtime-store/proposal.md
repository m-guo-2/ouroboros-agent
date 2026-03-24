## Why

当前 skill 系统虽然会从 GitHub 周期同步到本地文件夹，但运行时组装 prompt、解析技能索引和加载技能详情时仍主要依赖 GitHub store 的内存 cache。这样会导致启动冷窗、同步失败静默降级，以及 GitHub、内存、本地副本三套数据源并存，最终表现为 skill 明明已同步到本地却没有稳定进入运行时 prompt。

现在需要把 skill 的同步路径和运行路径彻底分离：GitHub 只负责分发，本地 DB 和本地文件夹负责实际服务。这样才能让 prompt 注入、`load_skill`、脚本执行都以本地持久副本为准，而不是依赖易失的内存状态。

## What Changes

- 将本地 `skills` 表提升为 skill 元数据的运行时唯一事实源，持久化 `id`、`name`、`description`、`enabled`、本地路径和同步时间等字段。
- 保留本地 skill 文件夹作为正文和资源存储层，`SKILL.md`、`scripts/`、`references/` 统一从本地读取。
- 将 GitHub skill store 重构为同步器：负责周期拉取、解析 frontmatter、落盘文件、upsert 本地 DB，而不再直接作为运行时读取源。
- 修改运行时 skill 读取链路：`GetSkillsContext()` 只查本地 DB；`GetSkillDetail()`、`load_skill_reference` 和 `run_script` 只读本地文件。
- 增加本地同步与运行诊断，显式区分“未绑定 skill”“本地未同步”“本地文件缺失”“skill 已禁用”等状态，避免静默跳过。

## Capabilities

### New Capabilities
- `skill-local-runtime-store`: 将 skill 元数据和文件副本落到本地，并要求运行时 prompt 注入、skill 详情读取和脚本执行统一以本地副本为准。

### Modified Capabilities

## Impact

- 后端：`agent/internal/github/skills.go`、`agent/internal/storage/skills.go`、`agent/internal/storage/db.go`、`agent/internal/runner/processor.go`
- 运行路径：`BuildSystemPrompt` 上游的 skill snippet 编译、`load_skill`、`load_skill_reference`、`run_script`
- 数据层：SQLite `skills` 表字段与同步写入逻辑
- 运维与诊断：启动后首次同步、手动 refresh、prompt 丢失时的排障可观测性
