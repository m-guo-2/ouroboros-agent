# VSCode 调试配置：channel-qiwei

- **日期**：2026-03-04
- **类型**：代码变更
- **状态**：已实施

## 背景

在联调企微回调时，需要快速进入 `channel-qiwei` 断点调试。仓库中尚无 `.vscode/launch.json`，每次调试都需要手动拼接运行参数和环境变量，效率较低。

## 决策

新增 VSCode Go 调试配置，固定 `program/cwd` 到 `channel-qiwei`，并自动加载 `channel-qiwei/.env`。

## 变更内容

- 新增 `.vscode/launch.json`
  - `name`: `Qiwei: Debug channel-qiwei`
  - `type`: `go`
  - `request`: `launch`
  - `program`: `${workspaceFolder}/channel-qiwei`
  - `cwd`: `${workspaceFolder}/channel-qiwei`
  - `envFile`: `${workspaceFolder}/channel-qiwei/.env`
  - `console`: `integratedTerminal`

## 影响

- 可直接通过 VSCode F5 启动企微服务调试，回调链路更易定位。
- 环境变量来源统一，减少“命令行能跑、调试不生效”的配置偏差。
