## ADDED Requirements

### Requirement: Session 具有 mode 状态字段
每个 SessionWorker SHALL 具有 `Mode` 字段，类型为 `SessionMode`，取值为 `"normal"` 或 `"plan"`。新建 session 的默认 mode SHALL 为 `"normal"`。

#### Scenario: 新 session 初始化
- **WHEN** 一个新的 SessionWorker 被创建
- **THEN** 其 Mode 值 SHALL 为 `"normal"`

#### Scenario: mode 可被设置为 plan
- **WHEN** enter_plan_mode 工具被调用
- **THEN** SessionWorker.Mode SHALL 变为 `"plan"`

#### Scenario: mode 可被恢复为 normal
- **WHEN** exit_plan_mode 工具被调用
- **THEN** SessionWorker.Mode SHALL 变为 `"normal"`

### Requirement: Mode 状态跨 processSession 调用持久化
SessionWorker 的 Mode 状态 SHALL 在 `processSession` 结束时持久化，并在下次 `processSession` 启动时恢复。用户可能间隔较长时间才回复，mode 不能丢失。

#### Scenario: plan 模式下 processSession 结束后恢复
- **WHEN** SessionWorker.Mode 为 `"plan"` 且 processSession 正常结束
- **THEN** 下次为同一 session 调用 processSession 时，SessionWorker.Mode SHALL 为 `"plan"`

#### Scenario: session worker 被回收后重建
- **WHEN** SessionWorker 因 idle timeout 被回收，之后同一 session 有新消息到达
- **THEN** 重建的 SessionWorker 的 Mode SHALL 从持久化存储中恢复
