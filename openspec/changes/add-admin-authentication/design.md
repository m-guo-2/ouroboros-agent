## Context

当前 agent 进程同时暴露管理 API 与 admin SPA：

- `agent/internal/api/router.go` 直接把 `/api/agent-sessions`、`/api/settings`、`/api/agents`、`/api/skills`、`/api/users`、`/api/traces`、`/api/services`、`/api/channels` 等管理接口挂到同一个 `ServeMux`。
- `agent/cmd/agent/main.go` 会在根路径 `/` 挂载 admin SPA，任意访问者都能直接进入后台。
- admin 前端的 `fetchApi()` 只是薄封装，没有登录态、401 处理、cookie/session 管理。

这意味着只要能访问服务端口，就能直接读写后台数据。该问题横跨配置、HTTP 中间层、接口设计、前端路由与部署流程，属于典型的跨模块安全改造。

## Goals / Non-Goals

**Goals:**
- 为 admin SPA 和管理 API 增加统一的访问控制边界。
- 提供最小但完整的登录流：登录、登出、会话探测、未认证重定向。
- 采用默认拒绝策略，避免因漏挂某个管理接口而留下裸露入口。
- 让部署显式提供管理员凭据和会话密钥，避免“开箱即裸奔”。

**Non-Goals:**
- 不做多用户、角色权限、RBAC、审计日志等高级权限系统。
- 不改业务通道接口（如 `/health`、渠道入站 webhook、发送通道消息 facade）的既有鉴权策略。
- 不引入独立身份服务或外部 OAuth 提供方。

## Decisions

### 1. 使用应用内登录页 + 签名 Cookie 会话，而不是 HTTP Basic Auth

采用单管理员账号登录页，`POST /api/admin-auth/login` 校验凭据后签发 HttpOnly Cookie；`POST /api/admin-auth/logout` 清理 Cookie；`GET /api/admin-auth/session` 返回当前会话状态。

选择原因：

- admin 已是 SPA，应用内登录页更容易和现有路由、401 状态、退出登录联动。
- 浏览器原生 Basic Auth 很难优雅退出，也不利于前端判断登录态。
- 服务端签名 Cookie 可保持实现简单，不需要额外 session 表。

会话设计：

- Cookie 仅包含最小声明信息，如用户名、签发时间、过期时间。
- Cookie 使用服务端密钥签名，设为 `HttpOnly`、`SameSite=Strict`，并在 HTTPS 场景启用 `Secure`。
- 会话设置固定过期时间；过期后前端重新跳转登录页。

替代方案：

- HTTP Basic Auth：实现最省，但浏览器 UX 差、登出困难、难与 SPA 状态管理融合。
- 服务端 session 存库：可随时吊销，但会引入新存储状态和清理逻辑；当前单管理员场景没有必要。
- JWT：对单体内部管理后台来说过重，且仍需处理前端存储与过期问题。

### 2. 管理接口通过统一保护层收口，默认拒绝未认证访问

为 admin 相关路由新增统一鉴权包装，而不是在每个 handler 内手写校验。实现上将 `api.Mount` 拆成公开路由与受保护路由两组，所有管理接口与 SPA 静态资源都走同一个认证判断。

保护范围：

- admin SPA 根路径与前端路由回退。
- 管理接口：`/api/agent-sessions*`、`/api/messages`、`/api/settings*`、`/api/agents*`、`/api/models*`、`/api/skills*`、`/api/users*`、`/api/traces*`、`/api/services`、`/api/channels`。
- 鉴权相关接口 ` /api/admin-auth/* ` 自身不受该保护，但其行为受登录逻辑约束。

保留公开的非后台接口：

- `/health`
- `/drain`（是否继续公开由实现时再评估，但本 change 不改变其现有控制方式）
- `/api/channels/incoming`
- `/api/data/channels/send`

替代方案：

- 逐 handler 添加认证：容易漏接口，长期维护风险高。
- 仅保护 SPA、不保护 API：前端被挡住但 API 仍可被直接调用，不能解决根因。

### 3. 管理员凭据采用显式配置，密码以哈希形式存储

新增 admin 鉴权配置段，例如：

- `admin_auth.enabled`
- `admin_auth.username`
- `admin_auth.password_hash`
- `admin_auth.session_secret`
- `admin_auth.session_ttl`

启动规则：

- 当挂载 admin SPA 或管理 API 时，若 `admin_auth.enabled=true`，必须同时提供合法的用户名、密码哈希和会话密钥。
- 若配置不完整，服务启动失败，避免以不安全默认值继续运行。

密码仅保存哈希，不保存明文。实现可使用 `bcrypt` 进行比对。这样即便部署配置被读取，也不会直接泄露后台密码。

替代方案：

- 明文密码配置：实现更简单，但高敏感凭据再次以明文形式散落到配置中，风险过高。
- 把账号密码放数据库：需要引入初始化流程与修改入口，超过当前需求范围。

### 4. 前端在路由层与请求层同时感知认证状态

admin 前端新增登录页和认证状态 store：

- 应用启动时请求 `GET /api/admin-auth/session`，确认是否已登录。
- 未登录访问受保护页面时，跳转到 `/login`。
- 任一 API 返回 401 时，清理本地认证状态并跳转登录页。
- 登录成功后回到原目标页面；登出后回到登录页。

请求层保持使用浏览器 Cookie，不把 token 存到 `localStorage`。这能减少凭据被脚本读取的风险，也避免自行拼接 `Authorization` 头。

替代方案：

- 只在路由层校验：首屏进入能挡住，但后台请求过期后的失败体验差。
- token 放 `localStorage`：前端接入简单，但更容易受到 XSS 影响。

## Risks / Trade-offs

- [Cookie 会话依赖浏览器同源策略] → 通过 `HttpOnly` + `SameSite=Strict` + 同源部署约束降低 CSRF 风险；后续若出现跨域部署需求，再补更强的 CSRF 机制。
- [单管理员方案扩展性有限] → 当前先解决“后台裸奔”的根问题；未来需要多用户时可在此边界上升级。
- [配置缺失导致启动失败] → 在部署模板和示例配置中同时补齐说明，避免上线时踩坑。
- [旧脚本直接调用管理 API 会失效] → 在 proposal 中已标记为 breaking，实施时补充登录方式与调用说明。

## Migration Plan

1. 在配置结构和部署模板中新增 `admin_auth` 字段，并生成管理员密码哈希。
2. 后端先增加登录接口、会话校验和管理路由保护层。
3. 前端接入登录页、会话探测、401 跳转与登出流程。
4. 预发布环境验证：未登录无法访问后台；登录后原有后台功能可正常工作。
5. 生产部署时配置管理员凭据与会话密钥；若回滚，恢复旧二进制和旧配置。

## Open Questions

- `/drain` 是否也应纳入同一套管理鉴权保护，而不是继续保持现状？
- 是否需要提供一个离线密码哈希生成脚本，降低运维手工出错概率？
