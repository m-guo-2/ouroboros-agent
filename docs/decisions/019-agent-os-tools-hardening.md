# Agent 默认 OS 工具集强化

- **日期**：2026-02-26
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

Agent 新增默认 OS 工具后，已具备基础的终端式操作能力，但评审中发现三个风险：

1) `workDir` 只更新内存，不会持久化到 session；
2) `grep` 对单文件只扫描前 512KB，大文件会漏结果；
3) `shell` 依赖固定 marker 解析 CWD，存在低概率输出碰撞。

这些问题会影响“像人使用控制台一样”的连续性与可靠性。

## 决策

对默认 OS 工具做一次稳态强化：持久化工作目录、将 `grep` 改为流式全量扫描、将 CWD marker 改为每次调用动态生成并稳健解析。

## 变更内容

- `agent/internal/runner/processor.go`
  - 在每轮结束后统一调用 `UpdateSession` 持久化 `workDir`。
  - 若有新上下文则同时写入 `context`，否则仅更新 `workDir`。
- `agent/internal/engine/ostools/ostools.go`
  - `shell`：
    - marker 从固定值改为“每次调用动态 marker（时间戳 + 随机数）”；
    - 提取逻辑改为 `parseMarkerCWD`，按 `\n<marker>\n<pwd>\n` 从尾部解析并清理输出。
  - `grep`：
    - 从“整文件读入 + 512KB 限制”改为“按行流式扫描”；
    - 维持 `before/after` 上下文能力，使用滑动窗口和 pending 列表；
    - 设置 scanner token 上限为 4MB，降低超长行报错概率。

## 考虑过的替代方案

- 方案 A：继续使用固定 marker，仅改成 stderr 输出。  
  否决原因：仍可能与用户 stderr 输出碰撞，且需要额外拆分 stderr 语义。
- 方案 B：`grep` 直接依赖系统 `grep` 命令。  
  否决原因：返回非结构化文本，不利于 LLM 稳定消费与后续工具链组合。

## 影响

- Shell CWD 在 worker 回收或服务重启后仍可延续，提升会话连续性。
- `grep` 在大文件场景下不再因 512KB 截断漏检，结果更可信。
- `shell` 输出解析鲁棒性提升，降低极端文本碰撞导致的误解析风险。
