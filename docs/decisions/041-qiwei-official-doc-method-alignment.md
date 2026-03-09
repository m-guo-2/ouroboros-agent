# 企微官方文档 Method 全量对齐

- **日期**：2026-03-04
- **类型**：代码变更
- **状态**：已实施

## 背景

`channel-qiwei` 早期 method 映射使用了内部抽象命名（如 `group/*`、`instance/*`），与 QiWe 官方文档页面示例 method（如 `/room/*`、`/client/*`）存在差异。  
在按官网文档联调时，这类差异会导致 API 代理调用失败或行为不一致。

## 决策

以 `https://doc.qiweapi.com/` 当前公开文档为唯一基准，对 `channel-qiwei` 的模块 action -> method 映射进行全量校正，并统一 method 规范为带前导 `/` 的官方路径。

## 变更内容

- 修改 `channel-qiwei/internal/modules/instance.go`
  - `create/resume/stop/set-callback` 对齐为 `/client/*`。
- 修改 `channel-qiwei/internal/modules/login.go`
  - 登录模块 action 对齐为 `/login/*` 最新命名。
- 修改 `channel-qiwei/internal/modules/contact.go`
  - 联系人相关 action 对齐为 `/contact/*` 文档路径（含外部/内部分页、搜索、同意申请、openid 等）。
- 修改 `channel-qiwei/internal/modules/group.go`
  - 群模块统一对齐为 `/room/*` 文档路径（分页、详情、成员管理、公告、群主转让、邀请确认等）。
- 修改 `channel-qiwei/internal/modules/message.go`
  - 消息模块对齐为 `/msg/*` 文档路径（含 `sendWeapp`、`sendPersonalCard`、`sendFeedVideo`、`statusModify`、群置顶与群发接口）。
- 修改 `channel-qiwei/internal/modules/cdn.go`
  - 云存储模块对齐为 `/cloud/*` 文档路径（上传、URL 上传、企微/个微下载及异步接口）。
- 修改 `channel-qiwei/internal/modules/moment.go`
  - 朋友圈模块对齐为 `/sns/*` 文档路径。
- 修改 `channel-qiwei/internal/modules/tag.go`
  - 标签模块对齐为 `/label/*` 文档路径。
- 修改 `channel-qiwei/internal/modules/session.go`
  - 会话模块对齐为 `/session/*` 文档路径。
- 修改 `channel-qiwei/api_handlers.go`
  - 出站发送接口默认 method 改为带 `/` 的官方路径。
- 修改 `channel-qiwei/qiwei_client.go`
  - 增加 `normalizeMethod`，兼容调用方传入不带 `/` 的旧写法。
- 修改 `channel-qiwei/events.go`
  - echo 模式发送文本改为 `/msg/sendText`。
  - 回调验活兼容新增 `testMsg/token` 形态识别，避免误判为异常 payload。

## 考虑过的替代方案

1. 保留旧映射，仅在 README 标注“存在别名”
   - 缺点：运行时仍可能调用失败，不能满足“按官方文档可直接联调”的目标。
2. 仅修复已暴露问题接口
   - 缺点：遗漏风险高，后续仍会出现零散不一致。

## 影响

- `channel-qiwei` 的模块代理接口与 QiWe 官网 method 命名保持一致，文档驱动联调成本显著降低。
- 旧调用方若传入不带 `/` method，仍可通过客户端规范化逻辑兼容。
- 回调地址验活日志噪音下降，消息订阅接入更稳定。
