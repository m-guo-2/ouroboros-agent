# 飞书独立服务 Action 路由重构

- **日期**：2026-03-02
- **类型**：代码变更
- **状态**：已实施

## 背景

`channel-feishu` 需要继续保持独立服务形态，并保留完整消息/群组/会议/文档能力。  
原 `src/routes/action.ts` 在单文件内同时承担 action 注册、参数转换、路由分发和分类推断，维护成本高，新增能力时容易引入回归。

## 决策

将 Action 元数据与处理逻辑从路由层抽离到独立注册表，路由仅负责协议校验和统一返回。

## 变更内容

- 新增 `channel-feishu/src/routes/action-registry.ts`
  - 引入 `ActionDefinition`（`name/category/handler`）统一描述 action
  - 将原有全部 action handler 迁移为显式注册表
  - 提供 `getActionDefinition`、`listActionNames` 等查询接口
- 重构 `channel-feishu/src/routes/action.ts`
  - 删除巨型内联 handler map，改为消费注册表
  - 保持 `POST /api/feishu/action` 与 `GET /api/feishu/action/list` 协议不变
  - `/list` 分类由“名称推断”改为“显式 category”
- 重构 `channel-feishu/src/routes/send.ts`
  - 提取 `sendMessage` / `replyMessage` 公共发送函数，消除重复 SDK 调用代码
  - 富文本消息构建时改为拷贝输入数组，避免对请求体原地修改
  - 保持新旧消息格式兼容路径不变（`SendRequest` + legacy `OutgoingMessage`）

## 考虑过的替代方案

- 方案 A：仅在 `action.ts` 内做注释和分区，不拆文件。  
  否决原因：结构问题仍在，后续 action 增长会继续放大维护风险。
- 方案 B：按领域拆成多个 action 路由（message/meeting/document）。  
  否决原因：短期会改变现有“统一 action 入口”心智和维护脚本，不符合本次“功能不变重构”的目标。

## 影响

- 对外 API 和 action 名称保持不变，调用方无需改造。
- `channel-feishu` 仍保持独立服务部署，不并入其他进程。
- 后续新增 action 时只需改注册表，降低回归风险并提升可读性。
