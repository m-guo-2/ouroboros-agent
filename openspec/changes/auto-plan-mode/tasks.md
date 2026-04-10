## 1. SessionMode 类型与状态字段

- [x] 1.1 在 `agent/internal/types/` 下定义 `SessionMode` 类型（`"normal"` / `"plan"`）
- [x] 1.2 在 `SessionWorker` struct 上增加 `Mode SessionMode` 字段，默认值 `"normal"`
- [x] 1.3 在 `processSession` 结束时将 mode 持久化（复用 `UpdateSession` 或 `UpdateSessionContextAndCursor` 路径）
- [x] 1.4 在 `processSession` 开始时从持久化存储恢复 mode 到 `SessionWorker.Mode`

## 2. Plan 模式工具注册

- [x] 2.1 创建 `enter_plan_mode` tool executor：设置 `worker.Mode = "plan"`，返回确认文本；已在 plan 模式时返回提示
- [x] 2.2 编写 `enter_plan_mode` 的 tool description（触发条件 + 不需要 plan 的场景）
- [x] 2.3 创建 `exit_plan_mode` tool executor：接收 `plan` 参数，通过 send_channel_message 路径发给用户，设置 `worker.Mode = "normal"`；非 plan 模式或 plan 为空时返回错误
- [x] 2.4 编写 `exit_plan_mode` 的 tool description
- [x] 2.5 在 `processSession` 的工具注册阶段调用两个工具的注册（与 `registerWecomBuiltinTools` 同级）

## 3. Plan 模式 Prompt 注入

- [x] 3.1 编写 plan 模式约束指令文本（只读工具列表、禁止副作用工具、使用 exit_plan_mode 提交计划）
- [x] 3.2 修改 `BuildSystemPrompt` 或 `processSession` 中构建 systemPrompt 的位置：当 `worker.Mode == "plan"` 时追加约束指令

## 4. 验证

- [ ] 4.1 手动测试：发送复杂任务，确认 LLM 调用 enter_plan_mode 进入计划模式
- [ ] 4.2 手动测试：plan 模式下 LLM 只使用只读工具，产出计划后调用 exit_plan_mode
- [ ] 4.3 手动测试：用户在微信回复确认后，LLM 按计划执行操作
- [ ] 4.4 手动测试：session worker idle 回收后重建，mode 能正确恢复
