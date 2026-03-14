# 共享 OSS 模块实现

- **日期**：2026-03-10
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

仓库当前由 `agent`、`channel-qiwei`、`channel-feishu` 三个独立 Go module 组成，如果把 OSS 能力直接放进某个服务内部，就不能真正复用。用户希望基于现有 MinIO 服务实现一个“共用模块”的上传下载能力，因此需要先解决共享落点问题，再实现具体客户端。

## 决策

新增独立的 `shared/oss` Go module，提供统一的对象上传、对象下载、配置读取、对象 key 生成和错误分类能力；MinIO 客户端作为底层实现封装在模块内部。

## 变更内容

- 新建 `shared/oss/`，并使用仓库远端路径作为 module path。
- 新增 `Config`、`Storage`、`PutObjectInput`、`PutObjectResult`、`GetObjectResult` 等公共接口与数据结构。
- 实现 `NewMinIOStorage`，封装 MinIO/S3 兼容客户端初始化、上传、下载和基于 context 的超时控制。
- 增加对象 key 生成策略，支持基于前缀、日期目录和随机后缀生成稳定 key。
- 增加统一错误分类，向上暴露 `ErrConfig`、`ErrAuthentication`、`ErrNotFound`、`ErrTransport`、`ErrInternal`。
- 提供 `FakeStorage` 作为测试替身，并补齐配置、key 生成、上传、下载、错误分类的单元测试。
- 增加 `shared/oss/README.md`，说明环境变量、示例代码和错误判断方式。
- MinIO 依赖选择 `github.com/minio/minio-go/v7 v7.0.98`，以保持与当前 Go 1.24 环境兼容。

## 考虑过的替代方案

### 放到 `agent/internal/storage`

没有采用。这样只能在 `agent` module 内使用，`channel-*` 侧无法直接复用，不符合“共用模块”的目标。

### 直接使用最新 `minio-go/v7`

没有采用。当前最新版要求 Go 1.25，而仓库环境仍是 Go 1.24，直接升级会扩大本次改动面。

## 影响

- 后续任一服务都可以把对象存储接入统一收敛到 `shared/oss`。
- 共享模块暂时还是独立存在，后续业务接入时需要在对应 module 增加依赖声明或 workspace 组织。
- 当前已具备稳定 API、测试替身和说明文档，适合作为后续媒体或附件场景的基础设施层。
