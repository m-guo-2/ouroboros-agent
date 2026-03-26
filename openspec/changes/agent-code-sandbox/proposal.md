## Why

AI Agent 需要执行 LLM 生成的代码（脚本运行、文件读写、依赖安装），这些代码本质上不可信。当前 Agent 只有宿主机上的 shell tool，直接执行存在安全风险。需要一套基于 Daytona 的隔离沙箱执行环境，提供安全的代码执行能力，同时保持 session 内环境状态共享。

## What Changes

- 新增 `SandboxManager`，管理 Daytona 沙箱的创建、复用与回收，采用 lazy per-session 生命周期
- 新增 4 个沙箱 Tool：`execute_command`、`write_file`、`read_file`、`list_files`，注册到 ToolRegistry
- 沙箱支持三层环境变量注入：镜像内置、创建时（session 级）、执行时（per-exec 临时）
- 沙箱支持可选的初始化流程（上传文件、安装依赖、自定义脚本）
- 后台巡检自动回收 idle 超时和 maxTTL 超时的沙箱
- graceful shutdown 时统一清理所有活跃沙箱

## Capabilities

### New Capabilities

- `sandbox-manager`: 沙箱生命周期管理，包含 lazy 创建、session 级复用、idle/maxTTL 自动回收、graceful shutdown
- `sandbox-tools`: 暴露给 LLM 的 4 个沙箱 Tool（execute_command、write_file、read_file、list_files），注册到现有 ToolRegistry

### Modified Capabilities

（无已有 spec 变更）

## Impact

- **agent/internal/sandbox/**: 新增 package，包含 SandboxManager、SessionSandbox、SandboxConfig 等核心类型
- **agent/internal/engine/registry.go**: ToolRegistry 需注册沙箱 Tool
- **agent/internal/runner/**: session 结束时通知 SandboxManager 主动回收沙箱
- **agent/internal/config/**: 新增沙箱相关配置项（Daytona API key、镜像、超时参数等）
- **go.mod**: 引入 Daytona Go SDK 依赖
- **Dockerfile**: 自定义沙箱镜像（预装 Python、Node.js 等高频工具链）
