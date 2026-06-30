## ADDED Requirements

### Requirement: execute_command tool
系统 SHALL 提供 `execute_command` Tool，在沙箱中执行 shell 命令。该 Tool SHALL 注册到 ToolRegistry，source 为 `builtin`，sourceName 为 `sandbox`。

#### Scenario: Execute simple command
- **WHEN** LLM 调用 `execute_command` 参数 `{"command": "echo hello"}`
- **THEN** 系统 SHALL 在当前 session 的沙箱中执行该命令
- **THEN** 返回结果 SHALL 包含 `success: true`、`exit_code: 0`、`stdout: "hello\n"`

#### Scenario: Command with non-zero exit code
- **WHEN** LLM 调用 `execute_command` 参数 `{"command": "python3 bad_script.py"}`
- **WHEN** 脚本执行返回 exit code 1
- **THEN** 返回结果 SHALL 包含 `success: false`、`exit_code: 1`
- **THEN** `stderr` SHALL 包含错误信息

#### Scenario: Command with custom environment variables
- **WHEN** LLM 调用 `execute_command` 参数 `{"command": "echo $MY_VAR", "env": {"MY_VAR": "test_value"}}`
- **THEN** 命令执行时 SHALL 注入 `MY_VAR=test_value` 环境变量
- **THEN** 该环境变量 SHALL 仅对本次执行有效，不影响后续命令

### Requirement: sandbox_set_env tool
系统 SHALL 提供 `sandbox_set_env` Tool，为当前 session 的沙箱持久注入或清除环境变量。持久 env SHALL 影响后续 `execute_command` 和 skill `run_script`，但 SHALL NOT 修改宿主进程环境或写入沙箱文件。

#### Scenario: Persistent env applied to later command
- **WHEN** LLM 调用 `sandbox_set_env` 参数 `{"env": {"API_BASE_URL": "https://example.test"}}`
- **WHEN** 后续调用 `execute_command` 参数 `{"command": "echo $API_BASE_URL"}`
- **THEN** 命令执行时 SHALL 读取到 `API_BASE_URL=https://example.test`

#### Scenario: Persistent env visible to skill scripts
- **WHEN** 当前 session 的沙箱已通过 `sandbox_set_env` 注入 `TOKEN=abc`
- **WHEN** LLM 调用 skill `run_script`
- **THEN** 脚本进程环境 SHALL 包含 `TOKEN=abc`

#### Scenario: Unset persistent env
- **WHEN** 当前 session 的沙箱已存在 `TOKEN=abc`
- **WHEN** LLM 调用 `sandbox_set_env` 参数 `{"unset": ["TOKEN"]}`
- **THEN** 后续命令和脚本环境 SHALL 不再包含 `TOKEN`

#### Scenario: Invalid env name rejected
- **WHEN** LLM 调用 `sandbox_set_env` 参数 `{"env": {"BAD-NAME": "x"}}`
- **THEN** Tool SHALL 返回明确错误
- **THEN** 当前沙箱 env SHALL 保持不变

#### Scenario: Command with custom timeout
- **WHEN** LLM 调用 `execute_command` 参数 `{"command": "sleep 10", "timeout": 5}`
- **THEN** 命令 SHALL 在 5 秒后被终止
- **THEN** 返回结果 SHALL 包含超时错误信息

#### Scenario: Default timeout applied
- **WHEN** LLM 调用 `execute_command` 未指定 timeout
- **THEN** 系统 SHALL 使用默认超时（60 秒）

#### Scenario: Timeout capped at maximum
- **WHEN** LLM 调用 `execute_command` 参数 `{"timeout": 600}`
- **THEN** 系统 SHALL 将 timeout 截断为最大值 300 秒

#### Scenario: Large output truncated
- **WHEN** 命令 stdout 超过 `MaxOutputBytes`（默认 10KB）
- **THEN** 返回结果 SHALL 截断输出，保留首尾各一半
- **THEN** 截断位置 SHALL 插入截断提示信息

### Requirement: write_file tool
系统 SHALL 提供 `write_file` Tool，向沙箱写入文件。

#### Scenario: Write file to workspace
- **WHEN** LLM 调用 `write_file` 参数 `{"path": "/workspace/script.py", "content": "print('hello')"}`
- **THEN** 系统 SHALL 在沙箱内创建 `/workspace/script.py`
- **THEN** 文件内容 SHALL 为 `print('hello')`
- **THEN** 返回结果 SHALL 包含 `success: true`

#### Scenario: Auto-create parent directories
- **WHEN** LLM 调用 `write_file` 参数 `{"path": "/workspace/sub/dir/file.txt", "content": "data"}`
- **WHEN** `/workspace/sub/dir/` 不存在
- **THEN** 系统 SHALL 自动创建所有缺失的父目录
- **THEN** 文件 SHALL 成功写入

#### Scenario: Overwrite existing file
- **WHEN** `/workspace/script.py` 已存在
- **WHEN** LLM 调用 `write_file` 写入同一路径
- **THEN** 原文件内容 SHALL 被覆盖

### Requirement: read_file tool
系统 SHALL 提供 `read_file` Tool，读取沙箱中的文件内容。

#### Scenario: Read existing file
- **WHEN** LLM 调用 `read_file` 参数 `{"path": "/workspace/output.txt"}`
- **WHEN** 文件存在
- **THEN** 返回结果 SHALL 包含文件完整内容

#### Scenario: Read non-existent file
- **WHEN** LLM 调用 `read_file` 参数 `{"path": "/workspace/not_found.txt"}`
- **WHEN** 文件不存在
- **THEN** 返回结果 SHALL 包含 `success: false` 和文件不存在的错误信息

### Requirement: list_files tool
系统 SHALL 提供 `list_files` Tool，列出沙箱中目录的文件和子目录。

#### Scenario: List workspace directory
- **WHEN** LLM 调用 `list_files` 参数 `{"dir": "/workspace"}`
- **THEN** 返回结果 SHALL 包含该目录下的文件和子目录列表

#### Scenario: Default directory
- **WHEN** LLM 调用 `list_files` 未指定 `dir`
- **THEN** 系统 SHALL 列出默认目录 `/workspace` 的内容

#### Scenario: List non-existent directory
- **WHEN** LLM 调用 `list_files` 参数 `{"dir": "/nonexistent"}`
- **THEN** 返回结果 SHALL 包含 `success: false` 和目录不存在的错误信息

### Requirement: Sandbox tools require sessionID in context
所有沙箱 Tool 的执行 SHALL 要求 context 中包含 sessionID。Runner 在调用 ToolRegistry.Execute 前 SHALL 将 sessionID 注入 context。

#### Scenario: SessionID present in context
- **WHEN** Runner 调用沙箱 Tool，context 中包含 sessionID
- **THEN** Tool SHALL 使用该 sessionID 获取对应沙箱并执行

#### Scenario: SessionID missing from context
- **WHEN** 沙箱 Tool 被调用但 context 中无 sessionID
- **THEN** Tool SHALL 返回明确错误："sandbox tool requires session context"

### Requirement: Sandbox tools registration conditional on API key
沙箱 Tool SHALL 仅在 Daytona API Key 配置就绪时注册到 ToolRegistry。

#### Scenario: API key configured
- **WHEN** 环境变量 `DAYTONA_API_KEY` 已设置
- **THEN** 沙箱 Tool SHALL 注册到 ToolRegistry
- **THEN** LLM 的 tool list 中 SHALL 包含这些 Tool

#### Scenario: API key not configured
- **WHEN** 环境变量 `DAYTONA_API_KEY` 未设置
- **THEN** 沙箱 Tool SHALL 不注册
- **THEN** LLM 的 tool list 中 SHALL 不包含沙箱 Tool
- **THEN** 系统其余 Tool 的行为 SHALL 不受影响
