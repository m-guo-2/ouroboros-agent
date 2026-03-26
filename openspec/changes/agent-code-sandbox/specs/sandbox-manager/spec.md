## ADDED Requirements

### Requirement: SandboxManager lazy per-session lifecycle
系统 SHALL 提供 `SandboxManager`，在每个 session 首次调用沙箱 Tool 时 lazy 创建 Daytona 沙箱，同一 session 内后续调用 SHALL 复用同一个沙箱实例。

#### Scenario: First sandbox tool call creates sandbox
- **WHEN** session `s1` 首次调用任一沙箱 Tool（如 execute_command）
- **THEN** SandboxManager SHALL 通过 Daytona SDK 创建一个新沙箱
- **THEN** 该沙箱 SHALL 绑定到 session `s1`
- **THEN** 沙箱创建时 SHALL 打上 `session_id: s1` label

#### Scenario: Subsequent tool calls reuse sandbox
- **WHEN** session `s1` 已有活跃沙箱
- **WHEN** `s1` 再次调用沙箱 Tool
- **THEN** SandboxManager SHALL 返回已有沙箱，不创建新的
- **THEN** `LastUsedAt` 时间戳 SHALL 更新为当前时间

#### Scenario: Different sessions get isolated sandboxes
- **WHEN** session `s1` 和 session `s2` 各自调用沙箱 Tool
- **THEN** 两个 session SHALL 各拥有独立的沙箱实例
- **THEN** 两个沙箱的文件系统和进程空间 SHALL 互相隔离

#### Scenario: No sandbox created when not needed
- **WHEN** session `s3` 全程只使用非沙箱 Tool（如 search_web）
- **THEN** SandboxManager SHALL 不为 `s3` 创建任何沙箱

### Requirement: Sandbox idle timeout recycling
SandboxManager SHALL 运行后台巡检 goroutine，周期性检查并回收 idle 超时的沙箱。

#### Scenario: Idle sandbox recycled
- **WHEN** 沙箱的 `LastUsedAt` 距当前时间超过 `IdleTimeout`（默认 15 分钟）
- **WHEN** 巡检 goroutine 执行检查
- **THEN** 该沙箱 SHALL 被销毁
- **THEN** session→sandbox 映射 SHALL 被移除
- **THEN** 系统 SHALL 记录回收日志，包含 session_id、回收原因和存活时长

#### Scenario: Active sandbox not recycled
- **WHEN** 沙箱的 `LastUsedAt` 距当前时间未超过 `IdleTimeout`
- **THEN** 巡检 SHALL 不回收该沙箱

### Requirement: Sandbox maxTTL forced recycling
沙箱从创建时刻起超过 `MaxTTL`（默认 2 小时）SHALL 被强制销毁，无论是否仍在使用。

#### Scenario: MaxTTL expired sandbox recycled
- **WHEN** 沙箱的 `CreatedAt` 距当前时间超过 `MaxTTL`
- **WHEN** 巡检 goroutine 执行检查
- **THEN** 该沙箱 SHALL 被强制销毁
- **THEN** 系统 SHALL 记录回收日志，reason 为 `ttl`

### Requirement: Session end triggers sandbox destruction
当 session 正常结束或异常终止时，Runner SHALL 通知 SandboxManager 销毁对应沙箱。

#### Scenario: Session ends normally
- **WHEN** Runner 检测到 session 结束（用户关闭或明确结束）
- **THEN** Runner SHALL 调用 `SandboxManager.Destroy(sessionID)`
- **THEN** 该 session 的沙箱 SHALL 被立即销毁

#### Scenario: Session has no sandbox
- **WHEN** session 结束时该 session 从未创建过沙箱
- **THEN** `Destroy` 调用 SHALL 安全返回，无副作用

### Requirement: Graceful shutdown cleanup
当 SandboxManager 所在进程退出时，SHALL 销毁所有活跃沙箱。

#### Scenario: Process graceful shutdown
- **WHEN** 进程收到终止信号开始 graceful shutdown
- **THEN** SandboxManager.Shutdown() SHALL 遍历并销毁所有活跃沙箱
- **THEN** 所有 session→sandbox 映射 SHALL 被清除

### Requirement: Sandbox configuration
SandboxManager SHALL 通过 `SandboxConfig` 配置，参数包含默认镜像、idle 超时、maxTTL、巡检间隔、命令执行默认超时、输出截断阈值。

#### Scenario: Custom configuration applied
- **WHEN** SandboxManager 以 `IdleTimeout=10min, MaxTTL=1h` 初始化
- **THEN** 巡检回收 SHALL 使用 10 分钟 idle 阈值和 1 小时 maxTTL 阈值

#### Scenario: Feature disabled when API key missing
- **WHEN** 环境变量 `DAYTONA_API_KEY` 未设置
- **THEN** SandboxManager SHALL 不启动
- **THEN** 沙箱 Tool SHALL 不注册到 ToolRegistry
- **THEN** 系统其余功能 SHALL 不受影响

### Requirement: Sandbox initialization
SandboxManager SHALL 支持可选的初始化流程，在沙箱首次创建后、执行用户任务前运行。

#### Scenario: Init with file uploads
- **WHEN** 沙箱创建时提供了 `InitRequest` 包含 `Files`
- **THEN** 系统 SHALL 将文件上传到沙箱的指定路径
- **THEN** 上传完成后沙箱 SHALL 进入就绪状态

#### Scenario: Init with Python requirements
- **WHEN** `InitRequest` 包含 `Requirements` 字段（pip requirements.txt 内容）
- **THEN** 系统 SHALL 在沙箱内执行 `pip install -r`
- **THEN** 安装失败 SHALL 返回错误

#### Scenario: No init request
- **WHEN** 沙箱创建时未提供 `InitRequest`
- **THEN** 沙箱 SHALL 直接进入就绪状态，不执行任何初始化

### Requirement: Concurrent access safety
SandboxManager SHALL 保证并发安全。多个 goroutine 可以同时调用 GetOrCreate 和 Destroy。

#### Scenario: Concurrent GetOrCreate for same session
- **WHEN** 两个 goroutine 同时为 session `s1` 调用 GetOrCreate
- **THEN** SHALL 只创建一个沙箱（通过 double-checked locking）
- **THEN** 两个 goroutine SHALL 获得同一个沙箱实例

#### Scenario: Serial command execution within sandbox
- **WHEN** 同一 session 的两个 tool call 并发到达
- **THEN** 沙箱内的命令执行 SHALL 串行化（通过 SessionSandbox 互斥锁）
- **THEN** 第二个命令 SHALL 等待第一个执行完成后再执行
