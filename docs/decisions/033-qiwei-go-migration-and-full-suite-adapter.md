# 企微独立服务 Go 全量迁移与模块化 API 适配

- **日期**：2026-03-04
- **类型**：架构决策 / 代码重构
- **状态**：已实施

## 背景

`channel-qiwei` 当前为 TS/Bun 实现，已具备基础的回调接收与消息回发能力，但存在以下问题：

1. **技术栈不一致**：`channel-feishu` 已迁移为 Go，企微仍是 TS，维护心智和运维链路割裂。
2. **能力边界偏窄**：现状主要覆盖消息主链路，未对 QiWe 平台 API 做系统化分域封装。
3. **可扩展性不足**：接口扩展依赖零散函数，缺少统一客户端、错误模型和重试策略。
4. **部署成本偏高**：引入 Bun 运行时，和主仓库 Go 服务族群不一致。

## 决策

### 总体决策

将 `channel-qiwei` **整体迁移到 Go**，并对 QiWe OpenAPI 做“统一客户端 + 分域模块”适配。

### 分层方案

1. **传输层**：`main.go` + `server.go`
   - 提供 `/health`、`/api/health`、`/webhook/callback`、`/api/qiwei/send`。
   - 新增统一模块代理路由：`/api/qiwei/{module}/{action}`。
2. **集成层**：`events.go` + `api_handlers.go`
   - 入站：回调快速 ACK，异步归一化并转发 Agent。
   - 出站：接收主服务回调并发送企微消息。
3. **平台客户端层**：`qiwei_client.go`
   - 统一封装 `/api/qw/doApi` 调用、超时、重试、错误映射。
4. **领域模块层**：`internal/modules/*`
   - 按 `instance/login/user/contact/group/message/cdn/moment/tag/session` 分域。
   - 每个模块暴露 action -> QiWe method 映射。

### 兼容策略

- 保留 TS 版既有主链路语义（回调字段映射、conversation 判断、Agent 转发格式）。
- 对高频消息发送接口提供显式 handler（便于后续增强）。
- 其余全量 API 通过模块路由统一代理，保证覆盖率与扩展速度。

## 变更内容

### 新增

- `channel-qiwei/go.mod`、`channel-qiwei/go.sum`
- `channel-qiwei/main.go`
- `channel-qiwei/config.go`
- `channel-qiwei/server.go`
- `channel-qiwei/models.go`
- `channel-qiwei/qiwei_client.go`
- `channel-qiwei/events.go`
- `channel-qiwei/api_handlers.go`
- `channel-qiwei/internal/modules/*`（10 个模块目录及映射实现）

### 修改

- `Makefile`
  - `qiwei` 目标从 `bun run dev` 切换到 `go run .`
  - `clean` 去除 TS 构建产物清理项
- `channel-qiwei/README.md`
  - 改为 Go 版运行、配置、接口说明
- `channel-qiwei/.env.example`
  - 补齐 Go 版所需环境变量
- `docs/decisions/README.md`
  - 追加本记录索引

### 下线

- `channel-qiwei/src/` 全部 TS 代码
- `channel-qiwei/package.json`
- `channel-qiwei/tsconfig.json`
- `channel-qiwei/bun.lock`

## 考虑过的替代方案

1. **Go/TS 双轨长期并行**
   - 优点：切换风险低。
   - 缺点：双实现长期维护，成本持续增加。
   - 结论：不采用，采用一次替换 + 分阶段联调。
2. **仅做 MVP（收发主链路）**
   - 优点：上线快。
   - 缺点：无法满足全量 API 诉求，后续重复改造。
   - 结论：不采用，直接建设全量分域适配层。
3. **继续 TS 并仅做结构整理**
   - 优点：短期改动小。
   - 缺点：与仓库整体 Go 化方向冲突。
   - 结论：不采用。

## 影响

- **正向影响**
  - 企微与飞书渠道服务技术栈统一，部署、诊断、升级路径一致。
  - 通过模块化代理快速覆盖 QiWe 全量 API，后续新增 action 只需扩映射表。
  - 回调链路与主服务联通保持稳定，兼顾可观测与可演进。
- **风险与约束**
  - 第三方文档变更会影响 method 映射，需要持续维护。
  - 若 QiWe 对单接口参数有特殊约束，需在模块层补显式校验。
  - 大批量调用需要结合限流与重试策略，避免放大故障。

## 验收标准

1. Go 服务可独立启动并通过健康检查。
2. 企微回调消息可被接收、归一化并转发到 Agent。
3. Agent 回调消息可通过 `/api/qiwei/send` 发送到企微。
4. 全量模块路由可覆盖 Instance/Login/User/Contact/Group/Message/CDN/Moment/Tag/Session。
5. TS/Bun 运行链路完全下线，`Makefile` 仅保留 Go 入口。
