## Context

当前 skill 系统已经具备 GitHub 周期同步、本地文件落盘和运行时按需加载等能力，但数据职责没有切清。`agent/internal/github/skills.go` 既负责从 GitHub 拉取 skill，又通过内存 cache 直接服务 `GetSkillsContext()`、`GetSkillDetail()` 等运行时调用；本地 `skills` 表仅保留兼容用途，本地 skill 文件夹虽然存在，却没有成为统一读取入口。

这种设计带来三个根因级问题：

1. 运行时依赖易失的内存 cache，进程启动后的首次会话可能在 cache ready 前就开始组装 prompt。
2. GitHub、内存、本地文件、SQLite 元数据并存，但没有明确 source of truth，导致排障时无法判断 skill 缺失到底发生在同步层还是运行层。
3. 运行链路和同步链路耦合，GitHub 不可用、同步失败或本地文件缺失时会表现为 skill 静默消失，而不是可诊断的本地状态问题。

相关决策文档已经明确了 skill 的两级加载和 per-agent 绑定语义，但没有把“运行时必须读本地持久副本”定成硬边界。本 change 负责补上这一层。

## Goals / Non-Goals

**Goals:**
- 让本地 `skills` 表成为 skill 元数据的运行时唯一事实源。
- 让本地 skill 文件夹成为 `SKILL.md`、`scripts/`、`references/` 的运行时唯一文件源。
- 将 GitHub store 收敛为同步器，负责远端拉取、解析、落盘和元数据 upsert，不直接参与在线 prompt 组装。
- 消除 skill prompt 注入的启动竞态，并让 skill 缺失能够被显式诊断。
- 保持现有 agent skill 绑定、`load_skill`、`load_skill_reference` 和 `run_script` 的用户语义不变。

**Non-Goals:**
- 不改变 skill 的 GitHub 仓库结构和 `SKILL.md` frontmatter 格式。
- 不重新设计 `SkillBinding`、always/on_demand 语义或 Admin UI 交互。
- 不引入新的远端存储或消息队列。
- 不在本 change 中处理 skill 的动态 tool 注册模型。

## Decisions

### D1. 本地 SQLite `skills` 表成为运行时元数据事实源

运行时凡是只需要 skill 元信息的地方，统一只查 SQLite，不再读取 GitHub store 的内存 cache。`skills` 表至少持久化以下字段：

- `id`
- `name`
- `description`
- `enabled`
- `metadata`，用于扩展保存 `base_path`、`scripts`、`references`、`synced_at`、`source_sha` 等同步状态
- `updated_at`

`GetSkillsContext()`、技能列表 API、agent full prompt 预览等一律从本地 DB 读取。

**为什么这样做：**
- prompt 注入只依赖小而稳定的 metadata，不应该被远端可用性或内存冷启动影响。
- SQLite 已经是本地运行态的持久层，适合承载可查询、可审计的 skill 状态。

**替代方案：**
- 继续以 GitHub cache 为主，DB 仅做备份。问题是启动竞态和双事实源依旧存在。
- 每次运行时扫描本地文件夹解析 frontmatter。这样省去 DB upsert，但查询成本、状态管理和诊断能力更差。

### D2. 本地 skill 文件夹成为正文与资源事实源

运行时凡是需要 skill 正文或文件资源的地方，统一从本地 skill 目录读取：

- `GetSkillDetail()` 从本地 `SKILL.md` 读取正文
- `GetSkillReference()` 从本地 `references/` 读取目标文件
- `run_script` 只执行本地 `scripts/` 下已同步的脚本

DB 负责告诉运行时“有哪些文件、位于哪里、当前是否可用”，真正内容由文件系统提供。

**为什么这样做：**
- `SKILL.md`、脚本和引用文件天然是文件资产，而不是关系型记录。
- 运行时直接读本地文件能把“skill 已同步但 cache 未热”与“skill 文件实际不存在”区分开。

**替代方案：**
- 把 readme 全文也放进 SQLite。这样可以减少一次文件读取，但会复制大文本、模糊脚本/引用文件的归属，并让 DB 承担不擅长的资产管理角色。

### D3. GitHub store 重构为“同步器”，不再是运行时读取源

`agent/internal/github/skills.go` 继续负责：

1. 周期拉取 GitHub `skills/`
2. 解析 `SKILL.md` frontmatter
3. 将 `SKILL.md`、`scripts/`、`references/` 同步到本地目录
4. 将 metadata upsert 到本地 `skills` 表

但它不再通过 `GetAll()` / `GetByID()` 为运行时 prompt 和 load_skill 提供主路径数据。内存 cache 可以保留为同步过程中的短时工作集，但不得再被视为 source of truth。

**为什么这样做：**
- 把同步面和服务面分离后，远端失败不会直接污染在线读取路径。
- 同步成功与否可以被编码为本地状态，而不是隐含在进程内存里。

### D4. 运行时 skill 缺失必须可诊断，不能静默跳过

对于以下情况，需要区分并显式返回或记录：

- agent 未绑定该 skill
- 本地 DB 中不存在该 skill metadata
- skill 已存在但 `enabled = false`
- 本地文件路径缺失或 `SKILL.md` / `scripts` / `references` 缺失

`GetSkillsContext()` 在构造 snippet 时不再把所有 miss 都当成普通忽略；至少要有结构化日志和诊断信息，供 full prompt 预览与运行排障使用。

**为什么这样做：**
- 当前最大问题不是单次失败，而是失败后用户只能看到“skill 没进 prompt”，系统内部却没有说明在哪一层丢了。

### D5. 同步采用“先落盘、再 upsert metadata、最后发布可见状态”的顺序

为避免 DB 记录先可见但本地文件尚未完成，单个 skill 的同步顺序定义为：

1. 拉取远端 `SKILL.md` 和资源文件
2. 写入本地 skill 目录
3. 校验关键文件存在
4. upsert 本地 `skills` 表

这样运行时即使正好读到新版本 skill，也能拿到与 metadata 对应的本地文件。

**替代方案：**
- 先写 DB 后落盘文件。实现更简单，但会暴露“DB 中存在、文件却尚未可读”的中间态，继续制造 prompt 与 load_skill 失配。

## Risks / Trade-offs

- **[Risk] 本地 DB 与本地文件仍可能发生偏离** → 通过同步顺序控制、文件存在校验和 refresh 诊断接口降低概率。
- **[Risk] 从 GitHub cache 切到本地读取会暴露历史脏数据** → 启动后执行一次全量 refresh，并在缺失时记录明确错误，尽早暴露问题。
- **[Trade-off] 同步逻辑更重，代码路径从单模块扩展到 DB + 文件系统** → 但这是用更明确的职责边界换取稳定运行态，复杂度是必要的。
- **[Trade-off] `load_skill` 首次读取需要访问本地文件而非内存字符串** → 代价是一次磁盘读取，但换来持久性和可诊断性，成本可接受。

## Migration Plan

1. 扩展本地 `skills` 表的 metadata 约定，支持保存本地路径与同步状态。
2. 在 GitHub 同步流程中加入 DB upsert，保证每次 refresh 后本地 DB 与本地文件同步更新。
3. 修改 `storage/skills.go` 的运行时读取路径，切断对 GitHub cache 的直接依赖。
4. 为 `load_skill`、`load_skill_reference`、`run_script` 增加本地缺失报错与日志。
5. 启动后执行一次同步并记录本地 skill 数量、缺失数量，作为回归验证。

回滚策略：
- 如新路径有严重问题，可临时切回旧版二进制，继续使用 GitHub cache 运行。
- 因为本 change 只增强本地副本，不改变 GitHub skill 仓库结构，回滚不会影响远端数据。

## Open Questions

- `skills.metadata` 是否足以承载 `base_path/scripts/references/synced_at/source_sha`，还是要拆成显式列。默认先使用 `metadata`，仅在查询需求增长时再拆列。
- 是否需要在 Admin UI 暴露“skill 本地同步状态”诊断视图。当前 change 先保证后端有可诊断日志和接口能力，不强制新增界面。
