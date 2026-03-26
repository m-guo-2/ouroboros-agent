## 1. 基础设施与依赖

- [ ] 1.1 在 `go.mod` 中引入 Daytona Go SDK 依赖
- [ ] 1.2 新建 `agent/internal/sandbox/` package 目录结构

## 2. SandboxConfig 与核心类型

- [ ] 2.1 定义 `SandboxConfig` 结构体（DefaultImage、IdleTimeout、MaxTTL、CleanupTick、ExecTimeout、MaxOutputBytes 等）
- [ ] 2.2 定义 `SessionSandbox` 结构体（SessionID、SandboxID、State、CreatedAt、LastUsedAt、sync.Mutex）
- [ ] 2.3 定义 `InitRequest` 结构体（Files、Requirements、NpmPackages、SetupScript）
- [ ] 2.4 实现配置加载逻辑：从环境变量读取 `DAYTONA_API_KEY` 和可选覆盖参数，API Key 缺失时返回 nil config 表示功能禁用

## 3. SandboxManager 生命周期管理

- [ ] 3.1 实现 `NewSandboxManager(config SandboxConfig, client *daytona.Client)` 构造函数
- [ ] 3.2 实现 `GetOrCreate(ctx, sessionID) (*SessionSandbox, error)` —— double-checked locking，创建时打 session_id label
- [ ] 3.3 实现 `Destroy(ctx, sessionID)` —— 主动销毁指定 session 的沙箱
- [ ] 3.4 实现 `initSandbox(ctx, sandbox, *InitRequest)` —— 上传文件、安装 pip/npm 依赖、执行自定义脚本
- [ ] 3.5 实现后台巡检 goroutine `startCleanupLoop` —— 周期性检查 idle 超时和 maxTTL 超时并回收
- [ ] 3.6 实现 `Shutdown(ctx)` —— graceful shutdown 时销毁所有活跃沙箱

## 4. Context 传递 sessionID

- [ ] 4.1 定义 `WithSessionID(ctx, sid)` 和 `SessionIDFromContext(ctx)` 辅助函数
- [ ] 4.2 在 `runner/processor.go` 中调用 ToolRegistry.Execute 前通过 `WithSessionID` 注入 sessionID 到 context

## 5. Sandbox Tool Handler

- [ ] 5.1 实现 `execute_command` tool handler —— 串行执行、超时控制、env 注入、输出截断
- [ ] 5.2 实现 `write_file` tool handler —— 自动创建父目录 + 上传内容
- [ ] 5.3 实现 `read_file` tool handler —— 读取沙箱文件内容
- [ ] 5.4 实现 `list_files` tool handler —— 列出目录内容，默认 /workspace
- [ ] 5.5 实现 `truncate` 辅助函数 —— 保留首尾各一半的截断策略

## 6. Tool 注册

- [ ] 6.1 定义 4 个 Tool 的 JSON Schema（name、description、parameters），与设计文档一致
- [ ] 6.2 在 Agent 启动流程中，当 DAYTONA_API_KEY 存在时创建 SandboxManager 并将 4 个 Tool 通过 `ToolRegistry.RegisterBuiltin` 注册
- [ ] 6.3 当 DAYTONA_API_KEY 缺失时跳过注册，不影响系统其余功能

## 7. Session 结束清理

- [ ] 7.1 在 Runner 的 session 结束路径中注入 `SandboxManager.Destroy(sessionID)` 调用（正常结束和异常终止均需覆盖）
- [ ] 7.2 SandboxManager 通过依赖注入传入 Runner

## 8. 沙箱镜像

- [ ] 8.1 创建 Dockerfile 定义默认沙箱镜像（基于 python:3.12-slim，预装 git/curl/jq/pandas/numpy/matplotlib/Node.js 22）

## 9. 验证

- [ ] 9.1 启动 Agent，确认 DAYTONA_API_KEY 已配置时 4 个沙箱 Tool 出现在 LLM tool list 中
- [ ] 9.2 通过 LLM 调用 write_file + execute_command 执行一个 Python 脚本，验证沙箱创建和命令执行正常
- [ ] 9.3 同一 session 多轮 tool call，验证文件系统状态在沙箱内持久保留
- [ ] 9.4 Session 结束后，验证沙箱被主动销毁
- [ ] 9.5 DAYTONA_API_KEY 未设置时启动 Agent，验证沙箱 Tool 不出现且系统正常运行
