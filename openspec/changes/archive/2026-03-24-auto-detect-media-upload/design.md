## Context

当前 `send_channel_message` 工具的媒体文件处理链路：

```
LLM 调用 send_channel_message(content, messageType)
  → resolveMediaContent(ctx, messageType, content)
    → isMediaMessageType(messageType)?  ── false → 直接返回 content 原文
                                         ── true  → 检测 URL/oss://path/本地路径 → 上传 OSS → presigned URL
  → channels.SendToChannel(outMsg)
```

问题在于 `isMediaMessageType` 是唯一的入口守卫。如果 LLM 没有传 `messageType: "image"`（默认 `text` 或为空），即使 content 是一个存在的本地图片路径，也会被当作纯文本直接下发给 channel。

此场景在 developer subagent 通过 shell 生成图片后尤为常见——subagent 返回本地路径，主 agent 不一定知道要用 image 类型发送。

## Goals / Non-Goals

**Goals:**
- 当 content 是本地文件路径且文件存在时，自动推断 `messageType` 并触发 OSS 上传，无需 LLM 显式指定
- 向后兼容：LLM 已经传了正确 messageType 的场景保持不变
- 自动推断仅在 messageType 为空或 `text` 时介入

**Non-Goals:**
- 不处理 HTTP URL 的类型推断（已有 URL 说明文件已在线上，channel 层自行处理）
- 不修改 channel 层的消息协议
- 不给 subagent 增加 OSS 上传工具
- 不涉及 oss:// URI 格式的自动类型推断（oss:// 已有专门处理路径）

## Decisions

### 1. 检测位置：在 `resolveMediaContent` 入口前，`send_channel_message` handler 内

**选择**：在 `send_channel_message` 的 handler 函数中，调用 `resolveMediaContent` 之前，插入文件检测 + messageType 自动修正逻辑。

**替代方案**：在 `resolveMediaContent` 内部修改入口逻辑。

**理由**：`resolveMediaContent` 的职责是"根据 messageType 处理媒体内容"，类型推断不属于它的职责。在 handler 中做类型修正更清晰——先确定 messageType，再交给 resolveMediaContent 处理。

### 2. 检测策略：`os.Stat` + 扩展名映射

**选择**：
1. 仅当 `messageType` 为空或 `text` 时触发自动检测
2. 对 content 做 `os.Stat`——文件存在则继续，不存在则跳过（不影响正常文本消息）
3. 根据文件扩展名映射到 messageType：
   - `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.bmp`, `.svg` → `image`
   - 其他存在的文件 → `file`
4. 修正后的 messageType 传给 `resolveMediaContent`

**替代方案**：读文件头做 MIME 检测（如 `http.DetectContentType`）。

**理由**：扩展名映射足够可靠且零 I/O 开销（仅一次 stat）。文件头检测需要 open + read，对于大文件会有不必要的开销，且实际场景中图片文件几乎总有正确扩展名。

### 3. 安全约束：仅对绝对路径或明确的相对路径触发

**选择**：content 必须看起来像一个文件路径（包含路径分隔符或以 `.` 开头）且不是 URL，才进行 `os.Stat`。对于纯文本消息内容（如 "hello world"），`os.Stat` 不会被调用。

**理由**：避免对正常文本消息做不必要的 stat 系统调用。一个简单的前置检查（不含 `://` 且满足路径特征）即可。

## Risks / Trade-offs

- **[风险] 文件名与文本消息冲突** → 概率极低。触发条件是 content 看起来像路径 + 文件实际存在 + messageType 为空/text。正常文本消息不会碰巧和宿主机上的文件路径重合。即使重合，stat 发现是目录或不可读文件时跳过即可。

- **[风险] stat 系统调用开销** → 单次 stat 耗时微秒级，可忽略。且仅在 messageType 为空/text 时执行，已有 messageType 的场景完全跳过。

- **[取舍] 不支持无扩展名文件的类型推断** → 无扩展名文件回退为 `file` 类型。如果 channel 层不支持通用 file 类型，会在 channel 层报错——与现有行为一致。
