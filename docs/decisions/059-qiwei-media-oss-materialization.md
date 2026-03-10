# 企微富媒体改为 OSS 物化

- **日期**：2026-03-10
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

`channel-qiwei` 原先会把图片、文件、视频先下载到本地临时目录，再把本地地址暴露给 agent 继续解析。仓库现在已经具备共享 `shared/oss` 模块，如果继续保留本地临时文件，就会让资源生命周期、跨进程可见性和后续扩展边界继续依赖单机文件系统。

## 决策

`channel-qiwei` 改为把文件类富媒体统一上传到共享 OSS，并向 agent 暴露 `oss://bucket/key` 形式的资源地址；`wecom_parse_message` 也改为优先接收 `resourceUri`，从 OSS 读取资源后再做图片识别或文本文件解析。

## 变更内容

- `channel-qiwei` 接入 `github.com/m-guo-2/ouroboros-agent/shared/oss` 并在 module 中声明本地 `replace`。
- 新增对象存储初始化与 URI 处理逻辑，统一生成和解析 `oss://bucket/key` 资源地址。
- 原先的附件下载逻辑不再落到 `os.TempDir()`，而是直接把下载流上传到 OSS。
- 图片解析从“读本地文件”改为“按资源地址从 OSS 读取对象内容，再转成 data URL”。
- 文本文件解析从“读本地文件”改为“按资源地址从 OSS 读取对象内容并提取文本”。
- `parse_message` 增加 `resourceUri` 入参，同时兼容旧的 `localPath`，便于 agent 平滑迁移。
- 回归测试改为基于 fake OSS 存储，而不是 `t.TempDir()`。

## 考虑过的替代方案

### 继续保留本地临时文件，只在后续场景中再接 OSS

没有采用。这样共享 OSS 模块无法真正落到首个调用方，媒体资源仍然依赖单机文件系统，不利于后续多实例或跨进程复用。

### 直接向 agent 暴露 MinIO HTTP 地址

没有采用。当前共享模块只承诺稳定的上传/下载能力，并没有预签名 URL 或公开访问地址语义。对 agent 暴露 `oss://bucket/key` 更稳定，也更容易在服务端工具内统一解析。

## 影响

- `channel-qiwei` 的文件类富媒体不再依赖本地临时目录作为持久化边界。
- agent 继续只拿到最小必要资源地址，但该地址从“本地路径”升级为“OSS 资源 URI”。
- 后续如果需要让其他服务或工具消费同一资源，只要复用 `shared/oss` 和 `oss://bucket/key` 协议即可。
