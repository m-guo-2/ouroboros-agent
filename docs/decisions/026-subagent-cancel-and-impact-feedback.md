# Subagent 取消与影响反馈

- **日期**：2026-02-28
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

subagent 已支持异步运行与完成通知，但主 Agent 仍缺少“中途取消”能力；同时在取消或失败后，主 Agent 需要知道子任务已经实际造成了哪些影响（例如已读/已写/已执行的操作）。

## 决策

1. 增加 `cancel_subagent` 工具，由主 Agent 主动发起取消。
2. subagent 在运行期间按工具调用记录影响（impact）。
3. 状态查询与完成通知中携带影响摘要，确保主 Agent 能基于“已发生事实”继续决策。

## 变更内容

- `agent/internal/subagent/manager.go`
  - 新增 `JobCanceled` 状态；
  - 新增 `Impact` 结构与 `Job.Impacts`；
  - 新增取消控制（`Cancel(jobID, reason)`）；
  - 工具执行增加影响记录包装（按工具调用生成 impact）；
  - 支持取消回调 `OnCanceled`。
- `agent/internal/runner/processor.go`
  - 新增 `cancel_subagent` 内建工具；
  - `get_subagent_status` 增加 `impacts` 与 `impactSummary`；
  - push 通知内容中追加“已产生影响”摘要。

## 影响

- 主 Agent 可在任务进行中主动止损。
- 即使子任务被取消或失败，主 Agent 也能看到已发生影响并做补偿动作。
- 后续可在 impact 上继续增强细粒度（例如 git diff、文件快照对比），不影响当前接口。
