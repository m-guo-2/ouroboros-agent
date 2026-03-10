# 企微多模态能力 Live 测试与文档理解补齐

- **日期**：2026-03-10
- **类型**：代码变更
- **状态**：已实施

## 背景

`channel-qiwei` 已经接入视觉识别、语音转写和附件二次解析，但此前只有语音带 live test，视觉缺少真实调用验证，文件理解底层也还停留在未实现状态。这样一来，测试无法回答“这三条能力链路现在到底能不能真正打通”。

## 决策

为视觉、语音、文件理解补齐可执行测试边界：视觉和文件理解新增 live integration test，三类 provider 新增纯单元测试；同时把文本类文档的 `ParseDocument` 真正接到 Ark 文档模型，避免文件理解永远停留在 stub。

## 变更内容

- 在 `channel-qiwei/facade_handlers.go` 中实现 `ParseDocument` 的文本类文档模型调用，支持 `.txt`、`.md`、`.json`、`.csv`、`.html`、`.xml` 等输入。
- 新增 `channel-qiwei/volc_ark_integration_test.go`，分别验证视觉模型与文档模型的真实调用结果非空。
- 新增 `channel-qiwei/volc_recognizer_test.go`，覆盖视觉、文档、语音三类 provider 的请求构造、鉴权头与返回解析。
- 复用现有 `TestParseMessageLocalFileReadsText`，继续保证本地文本文件解析路径可回归。

## 考虑过的替代方案

### 只跑现有语音 live test，不补视觉和文件理解

没有采用。这样只能证明一条链路，无法回答用户关心的“三种能力分别是否可用”。

### 只补测试，不实现文件理解

没有采用。`ParseDocument` 之前直接返回未实现，测试只能得到恒定失败，不能验证真实链路。

## 影响

- 现在可以区分“代码调用契约正常”与“外部 provider 凭证/授权异常”两类问题。
- 视觉模型与文本类文件理解已经有真实回归入口，可直接用于环境验收。
- 二进制文档理解仍未覆盖；本次只补齐文本类文档走模型的能力，后续若要支持 PDF/Office，需要增加文档抽取或专用解析链路。
