/**
 * API 代理 — Anthropic Messages API ↔ OpenAI Chat Completions API 格式转换
 *
 * 从旧版 orchestrator 迁移。
 * 关键变化：配置通过参数传入，不使用全局状态或 env。
 *
 * 路由：
 *   POST /v1/messages                      → 全局代理（旧，保留兼容）
 *   POST /s/:sessionId/v1/messages         → Session 级代理（新，支持 model I/O 观测）
 *   POST /v1/messages/count_tokens         → 简单估算 token 数
 *   GET  /v1/health                        → 代理健康检查
 */

import { Router, type Request, type Response } from "express";
import type { ModelConfig } from "./server-client";

// ============================================================
// 全局代理状态（旧接口，保留兼容）
// ============================================================

let currentTarget: { baseUrl: string; apiKey: string; model: string } = {
  baseUrl: "",
  apiKey: "",
  model: "",
};

/**
 * 更新代理目标（旧接口，由 sdk-runner 在每次请求前调用）
 */
export function updateProxyTarget(config: { baseUrl: string; apiKey: string; model: string }): void {
  currentTarget = { ...config };
}

export function getProxyTarget(): { baseUrl: string; apiKey: string; model: string } {
  return { ...currentTarget };
}

// ============================================================
// Session 级代理注册（新接口，支持并发 session 隔离）
// ============================================================

export interface SessionProxyConfig {
  baseUrl: string;
  apiKey: string;
  model: string;
  /** true = Anthropic 原生 API，false = OpenAI-compat（需格式转换） */
  isAnthropicNative: boolean;
  /** 模型 I/O 回调：每次模型完成响应后触发 */
  onModelIO?: (input: unknown, output: unknown) => void;
}

const sessionProxies = new Map<string, SessionProxyConfig>();

export function registerSessionProxy(sessionId: string, config: SessionProxyConfig): void {
  sessionProxies.set(sessionId, config);
}

export function unregisterSessionProxy(sessionId: string): void {
  sessionProxies.delete(sessionId);
}

// ============================================================
// 模型 I/O 摘要构建工具
// ============================================================

/** 从 Anthropic 格式请求体中提取输入摘要（裁剪过大数据） */
function buildModelInputSummary(body: any): unknown {
  let systemPrompt: string | undefined;
  if (typeof body.system === "string") {
    systemPrompt = body.system.substring(0, 1000);
  } else if (Array.isArray(body.system)) {
    systemPrompt = body.system
      .filter((b: any) => b.type === "text")
      .map((b: any) => (b.text || "") as string)
      .join("\n")
      .substring(0, 1000);
  }

  const messages = ((body.messages || []) as any[]).map((m: any) => {
    let contentSummary = "";
    if (typeof m.content === "string") {
      contentSummary = m.content.substring(0, 400);
    } else if (Array.isArray(m.content)) {
      contentSummary = m.content
        .map((b: any) => {
          if (b.type === "text") return (b.text || "").substring(0, 300);
          if (b.type === "tool_result") {
            const r = typeof b.content === "string" ? b.content : JSON.stringify(b.content || "");
            return `[tool_result:${b.tool_use_id}] ${r.substring(0, 200)}`;
          }
          if (b.type === "tool_use") return `[tool_use:${b.name}]`;
          return "";
        })
        .filter(Boolean)
        .join(" | ")
        .substring(0, 400);
    }
    return { role: m.role, content: contentSummary };
  });

  return {
    model: body.model,
    systemPrompt,
    messageCount: (body.messages || []).length,
    messages,
    toolNames: ((body.tools || []) as any[]).map((t: any) => t.name).filter(Boolean),
  };
}

/** 从 Anthropic 格式响应体中提取输出摘要 */
function buildModelOutputSummary(response: any): unknown {
  const content = (response.content || []) as any[];
  const textContent = content
    .filter((b: any) => b.type === "text")
    .map((b: any) => b.text || "")
    .join("")
    .substring(0, 2000);
  const toolCalls = content
    .filter((b: any) => b.type === "tool_use")
    .map((b: any) => ({
      name: b.name,
      id: b.id,
      inputSummary: JSON.stringify(b.input || {}).substring(0, 300),
    }));
  return {
    content: textContent,
    toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
    stopReason: response.stop_reason,
    inputTokens: response.usage?.input_tokens,
    outputTokens: response.usage?.output_tokens,
  };
}

// ============================================================
// Anthropic SSE 流式响应累积器
// ============================================================

interface AnthropicStreamBlock {
  type: string;
  text: string;
  id?: string;
  name?: string;
  inputJson: string;
}

interface AnthropicStreamAccum {
  blocks: Map<number, AnthropicStreamBlock>;
  stopReason?: string;
  inputTokens: number;
  outputTokens: number;
}

function processAnthropicSSEEvent(event: any, accum: AnthropicStreamAccum): void {
  switch (event.type) {
    case "message_start":
      accum.inputTokens = event.message?.usage?.input_tokens || 0;
      break;
    case "content_block_start":
      accum.blocks.set(event.index, {
        type: event.content_block?.type || "text",
        text: event.content_block?.text || "",
        id: event.content_block?.id,
        name: event.content_block?.name,
        inputJson: "",
      });
      break;
    case "content_block_delta": {
      const block = accum.blocks.get(event.index);
      if (block) {
        if (event.delta?.type === "text_delta") {
          block.text += event.delta.text || "";
        } else if (event.delta?.type === "input_json_delta") {
          block.inputJson += event.delta.partial_json || "";
        }
      }
      break;
    }
    case "message_delta":
      accum.stopReason = event.delta?.stop_reason;
      accum.outputTokens = event.usage?.output_tokens || 0;
      break;
  }
}

function buildOutputSummaryFromAccum(accum: AnthropicStreamAccum): unknown {
  const textContent = Array.from(accum.blocks.values())
    .filter((b) => b.type === "text")
    .map((b) => b.text)
    .join("")
    .substring(0, 2000);
  const toolCalls = Array.from(accum.blocks.values())
    .filter((b) => b.type === "tool_use")
    .map((b) => ({
      name: b.name,
      id: b.id,
      inputSummary: b.inputJson.substring(0, 300),
    }));
  return {
    content: textContent,
    toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
    stopReason: accum.stopReason,
    inputTokens: accum.inputTokens,
    outputTokens: accum.outputTokens,
  };
}

// ============================================================
// Anthropic 原生请求处理（直接转发，不做格式转换）
// ============================================================

async function handleAnthropicNativeRequest(
  req: Request,
  res: Response,
  config: SessionProxyConfig,
): Promise<void> {
  const body = req.body;
  const isStream = body.stream === true;
  const targetUrl = `${config.baseUrl}/v1/messages`;
  const inputSummary = buildModelInputSummary(body);

  const forwardHeaders: Record<string, string> = {
    "Content-Type": "application/json",
    "x-api-key": config.apiKey,
    "anthropic-version": (req.headers["anthropic-version"] as string) || "2023-06-01",
  };
  if (req.headers["anthropic-beta"]) {
    forwardHeaders["anthropic-beta"] = req.headers["anthropic-beta"] as string;
  }

  let upstream: globalThis.Response;
  try {
    upstream = await fetch(targetUrl, {
      method: "POST",
      headers: forwardHeaders,
      body: JSON.stringify(body),
    });
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    console.error(`[proxy-session] Anthropic native fetch error: ${msg}`);
    res.status(502).json({ type: "error", error: { type: "api_error", message: msg } });
    return;
  }

  if (!upstream.ok) {
    const errorText = await upstream.text();
    console.error(`[proxy-session] Anthropic error ${upstream.status}: ${errorText.substring(0, 300)}`);
    res.status(upstream.status).json({
      type: "error",
      error: { type: "api_error", message: `Upstream ${upstream.status}: ${errorText}` },
    });
    return;
  }

  if (!isStream) {
    const response = await upstream.json() as any;
    config.onModelIO?.(inputSummary, buildModelOutputSummary(response));
    res.json(response);
    return;
  }

  // SSE 流式：转发 + 同时累积内容
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");

  const accum: AnthropicStreamAccum = { blocks: new Map(), inputTokens: 0, outputTokens: 0 };
  const reader = upstream.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      const chunk = decoder.decode(value, { stream: true });
      res.write(chunk);

      buffer += chunk;
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (!line.startsWith("data: ")) continue;
        const dataStr = line.slice(6).trim();
        if (!dataStr || dataStr === "[DONE]") continue;
        try {
          processAnthropicSSEEvent(JSON.parse(dataStr), accum);
        } catch { /* skip */ }
      }
    }
  } catch (err) {
    console.error("[proxy-session] Anthropic native stream error:", err);
  }

  config.onModelIO?.(inputSummary, buildOutputSummaryFromAccum(accum));
  res.end();
}

// ============================================================
// OpenAI-compat 请求处理（格式转换，复用旧逻辑）
// ============================================================

async function handleOpenAICompatRequest(
  req: Request,
  res: Response,
  config: SessionProxyConfig,
): Promise<void> {
  const body = req.body;
  const isStream = body.stream === true;
  const openAIBody = toOpenAIRequest(body, config.model);
  const targetUrl = `${config.baseUrl}/v1/chat/completions`;
  const inputSummary = buildModelInputSummary(body);

  console.log(`   → [Proxy-Session] ${isStream ? "Stream" : "Sync"} → ${config.model}`);

  let upstream: globalThis.Response;
  try {
    upstream = await fetch(targetUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${config.apiKey}` },
      body: JSON.stringify(openAIBody),
    });
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    res.status(502).json({ type: "error", error: { type: "api_error", message: msg } });
    return;
  }

  if (!upstream.ok) {
    const errorText = await upstream.text();
    console.error(`   ✗ [Proxy-Session] Upstream ${upstream.status}: ${errorText.substring(0, 200)}`);
    res.status(upstream.status).json({ type: "error", error: { type: "api_error", message: `Upstream ${upstream.status}: ${errorText}` } });
    return;
  }

  const model = body.model || config.model;

  if (!isStream) {
    const oaiRes = await upstream.json() as any;
    const anthropicRes = toAnthropicResponse(oaiRes, model);
    config.onModelIO?.(inputSummary, buildModelOutputSummary(anthropicRes));
    res.json(anthropicRes);
    return;
  }

  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");

  const streamState: StreamState = { started: false, blockIndex: 0, textBlockOpen: false, toolCalls: new Map(), inputTokens: 0, outputTokens: 0 };
  // For I/O hook accumulation
  const accumBlocks = new Map<number, AnthropicStreamBlock>();
  let accumStopReason: string | undefined;

  const reader = upstream.body!.getReader();
  const decoder = new TextDecoder();

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        if (streamState.started) {
          if (streamState.textBlockOpen) res.write(makeSSE("content_block_stop", { type: "content_block_stop", index: streamState.blockIndex }));
          res.write(makeSSE("message_delta", { type: "message_delta", delta: { stop_reason: "end_turn" }, usage: { output_tokens: streamState.outputTokens } }));
          res.write(makeSSE("message_stop", { type: "message_stop" }));
        }
        break;
      }
      const text = decoder.decode(value, { stream: true });
      for (const line of text.split("\n")) {
        const trimmed = line.trim();
        if (!trimmed || trimmed === "data: [DONE]" || !trimmed.startsWith("data: ")) continue;
        try {
          const chunk = JSON.parse(trimmed.slice(6));
          // Accumulate for I/O hook
          accumulateOAIChunk(chunk, accumBlocks, model, (reason) => { accumStopReason = reason; });
          const sseOutput = processStreamChunk(chunk, streamState, model);
          if (sseOutput) res.write(sseOutput);
        } catch { /* skip */ }
      }
    }
  } catch (err) {
    console.error("   ✗ [Proxy-Session] Stream error:", err);
  }

  const accum: AnthropicStreamAccum = { blocks: accumBlocks, stopReason: accumStopReason, inputTokens: streamState.inputTokens, outputTokens: streamState.outputTokens };
  config.onModelIO?.(inputSummary, buildOutputSummaryFromAccum(accum));
  res.end();
}

/** 累积 OpenAI 流式 chunks 用于 I/O hook */
function accumulateOAIChunk(
  chunk: any,
  blocks: Map<number, AnthropicStreamBlock>,
  _model: string,
  onFinish: (reason: string) => void,
): void {
  const delta = chunk.choices?.[0]?.delta;
  const finishReason = chunk.choices?.[0]?.finish_reason;

  if (delta?.content) {
    const textBlock = blocks.get(0) || { type: "text", text: "", inputJson: "" };
    textBlock.text += delta.content;
    blocks.set(0, textBlock);
  }
  if (delta?.tool_calls) {
    for (const tc of delta.tool_calls) {
      const idx = (tc.index ?? 0) + 1000; // offset to avoid collision with text block
      let block = blocks.get(idx);
      if (tc.id && tc.function?.name) {
        block = { type: "tool_use", text: "", id: tc.id, name: tc.function.name, inputJson: "" };
        blocks.set(idx, block);
      }
      if (block && tc.function?.arguments) {
        block.inputJson += tc.function.arguments;
      }
    }
  }
  if (finishReason) onFinish(finishReason);
}

// ============================================================
// 请求转换：Anthropic → OpenAI
// ============================================================

function convertTools(tools: any[]): any[] {
  return tools.map((t) => ({
    type: "function",
    function: {
      name: t.name,
      description: t.description || "",
      parameters: t.input_schema || { type: "object", properties: {} },
    },
  }));
}

function convertMessages(messages: any[], system?: any): any[] {
  const result: any[] = [];

  if (system) {
    const text =
      typeof system === "string"
        ? system
        : Array.isArray(system)
          ? system.map((b: any) => b.text || "").filter(Boolean).join("\n")
          : "";
    if (text) result.push({ role: "system", content: text });
  }

  for (const msg of messages) {
    if (msg.role === "user") {
      if (typeof msg.content === "string") {
        result.push({ role: "user", content: msg.content });
        continue;
      }
      for (const block of msg.content) {
        if (block.type === "tool_result") {
          result.push({
            role: "tool",
            tool_call_id: block.tool_use_id,
            content: typeof block.content === "string" ? block.content : JSON.stringify(block.content),
          });
        } else if (block.type === "text") {
          result.push({ role: "user", content: block.text });
        }
      }
    } else if (msg.role === "assistant") {
      if (typeof msg.content === "string") {
        result.push({ role: "assistant", content: msg.content });
        continue;
      }
      const textParts: string[] = [];
      const toolCalls: any[] = [];
      for (const block of msg.content) {
        if (block.type === "text") textParts.push(block.text);
        else if (block.type === "tool_use") {
          toolCalls.push({
            id: block.id,
            type: "function",
            function: {
              name: block.name,
              arguments: typeof block.input === "string" ? block.input : JSON.stringify(block.input),
            },
          });
        }
      }
      const assistantMsg: any = { role: "assistant", content: textParts.length > 0 ? textParts.join("\n") : null };
      if (toolCalls.length > 0) assistantMsg.tool_calls = toolCalls;
      result.push(assistantMsg);
    }
  }

  return result;
}

function toOpenAIRequest(body: any, targetModel: string): any {
  const req: any = {
    model: targetModel,
    messages: convertMessages(body.messages, body.system),
    max_tokens: body.max_tokens || 4096,
    stream: body.stream || false,
  };
  if (body.tools?.length > 0) req.tools = convertTools(body.tools);
  if (body.temperature !== undefined) req.temperature = body.temperature;
  if (body.top_p !== undefined) req.top_p = body.top_p;
  if (req.stream) req.stream_options = { include_usage: true };
  return req;
}

// ============================================================
// 响应转换：OpenAI → Anthropic（非流式）
// ============================================================

function toAnthropicResponse(oaiRes: any, model: string): any {
  const choice = oaiRes.choices?.[0];
  const content: any[] = [];

  if (choice?.message?.content) content.push({ type: "text", text: choice.message.content });
  if (choice?.message?.tool_calls) {
    for (const tc of choice.message.tool_calls) {
      let input: any = {};
      try { input = JSON.parse(tc.function.arguments); } catch { input = { raw: tc.function.arguments }; }
      content.push({ type: "tool_use", id: tc.id, name: tc.function.name, input });
    }
  }
  if (content.length === 0) content.push({ type: "text", text: "" });

  const stopMap: Record<string, string> = { stop: "end_turn", tool_calls: "tool_use", length: "max_tokens" };
  return {
    id: `msg_${oaiRes.id || Date.now()}`,
    type: "message",
    role: "assistant",
    content,
    model,
    stop_reason: stopMap[choice?.finish_reason] || "end_turn",
    usage: { input_tokens: oaiRes.usage?.prompt_tokens || 0, output_tokens: oaiRes.usage?.completion_tokens || 0 },
  };
}

// ============================================================
// 流式转换：OpenAI SSE → Anthropic SSE
// ============================================================

interface StreamState {
  started: boolean;
  blockIndex: number;
  textBlockOpen: boolean;
  toolCalls: Map<number, { id: string; name: string; blockIndex: number; started: boolean }>;
  inputTokens: number;
  outputTokens: number;
}

function makeSSE(event: string, data: any): string {
  return `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
}

function processStreamChunk(chunk: any, state: StreamState, model: string): string {
  let output = "";

  if (!state.started) {
    state.started = true;
    output += makeSSE("message_start", {
      type: "message_start",
      message: { id: `msg_${chunk.id || Date.now()}`, type: "message", role: "assistant", content: [], model, stop_reason: null, usage: { input_tokens: 0, output_tokens: 0 } },
    });
  }

  const delta = chunk.choices?.[0]?.delta;
  const finishReason = chunk.choices?.[0]?.finish_reason;

  if (delta?.content) {
    if (!state.textBlockOpen) {
      state.textBlockOpen = true;
      output += makeSSE("content_block_start", { type: "content_block_start", index: state.blockIndex, content_block: { type: "text", text: "" } });
    }
    output += makeSSE("content_block_delta", { type: "content_block_delta", index: state.blockIndex, delta: { type: "text_delta", text: delta.content } });
  }

  if (delta?.tool_calls) {
    for (const tc of delta.tool_calls) {
      const idx = tc.index ?? 0;
      let tool = state.toolCalls.get(idx);
      if (tc.id && tc.function?.name) {
        if (state.textBlockOpen) {
          output += makeSSE("content_block_stop", { type: "content_block_stop", index: state.blockIndex });
          state.blockIndex++;
          state.textBlockOpen = false;
        }
        tool = { id: tc.id, name: tc.function.name, blockIndex: state.blockIndex, started: false };
        state.toolCalls.set(idx, tool);
      }
      if (!tool) continue;
      if (!tool.started) {
        tool.started = true;
        output += makeSSE("content_block_start", { type: "content_block_start", index: tool.blockIndex, content_block: { type: "tool_use", id: tool.id, name: tool.name, input: {} } });
      }
      if (tc.function?.arguments) {
        output += makeSSE("content_block_delta", { type: "content_block_delta", index: tool.blockIndex, delta: { type: "input_json_delta", partial_json: tc.function.arguments } });
      }
    }
  }

  if (chunk.usage) {
    state.inputTokens = chunk.usage.prompt_tokens || 0;
    state.outputTokens = chunk.usage.completion_tokens || 0;
  }

  if (finishReason) {
    if (state.textBlockOpen) {
      output += makeSSE("content_block_stop", { type: "content_block_stop", index: state.blockIndex });
      state.blockIndex++;
      state.textBlockOpen = false;
    }
    for (const [, tool] of state.toolCalls) {
      if (tool.started) {
        output += makeSSE("content_block_stop", { type: "content_block_stop", index: tool.blockIndex });
        state.blockIndex = Math.max(state.blockIndex, tool.blockIndex + 1);
      }
    }
    const stopMap: Record<string, string> = { stop: "end_turn", tool_calls: "tool_use", length: "max_tokens" };
    output += makeSSE("message_delta", { type: "message_delta", delta: { stop_reason: stopMap[finishReason] || "end_turn" }, usage: { output_tokens: state.outputTokens } });
    output += makeSSE("message_stop", { type: "message_stop" });
  }

  return output;
}

// ============================================================
// Express Router
// ============================================================

export function createProxyRouter(): Router {
  const router = Router();

  router.get("/health", (_req: Request, res: Response) => {
    res.json({ status: "ok", target: currentTarget.baseUrl, model: currentTarget.model, activeSessions: sessionProxies.size });
  });

  router.post("/messages/count_tokens", async (req: Request, res: Response) => {
    const text = JSON.stringify(req.body.messages);
    res.json({ input_tokens: Math.ceil(text.length / 4) });
  });

  // ── Session 级代理（新，并发安全）──────────────────────────────
  router.post("/s/:sessionId/messages", async (req: Request, res: Response) => {
    const { sessionId } = req.params;
    const config = sessionProxies.get(sessionId);

    if (!config) {
      console.error(`[proxy-session] Session proxy not found: ${sessionId}`);
      return res.status(502).json({
        type: "error",
        error: { type: "api_error", message: `Session proxy not registered: ${sessionId}` },
      });
    }

    try {
      if (config.isAnthropicNative) {
        await handleAnthropicNativeRequest(req, res, config);
      } else {
        await handleOpenAICompatRequest(req, res, config);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Unknown proxy error";
      console.error(`[proxy-session] Error for session ${sessionId}: ${msg}`);
      if (!res.headersSent) {
        res.status(500).json({ type: "error", error: { type: "api_error", message: msg } });
      }
    }
  });

  // ── 全局代理（旧，向后兼容）────────────────────────────────────
  router.post("/messages", async (req: Request, res: Response) => {
    try {
      const body = req.body;
      const isStream = body.stream === true;
      const openAIBody = toOpenAIRequest(body, currentTarget.model);
      const targetUrl = `${currentTarget.baseUrl}/v1/chat/completions`;

      console.log(`   → [Proxy] ${isStream ? "Stream" : "Sync"} → ${currentTarget.model}`);

      const upstream = await fetch(targetUrl, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${currentTarget.apiKey}` },
        body: JSON.stringify(openAIBody),
      });

      if (!upstream.ok) {
        const errorText = await upstream.text();
        console.error(`   ✗ [Proxy] Upstream ${upstream.status}: ${errorText.substring(0, 200)}`);
        res.status(upstream.status).json({ type: "error", error: { type: "api_error", message: `Upstream ${upstream.status}: ${errorText}` } });
        return;
      }

      if (!isStream) {
        const oaiRes = await upstream.json();
        res.json(toAnthropicResponse(oaiRes, body.model || currentTarget.model));
        return;
      }

      res.setHeader("Content-Type", "text/event-stream");
      res.setHeader("Cache-Control", "no-cache");
      res.setHeader("Connection", "keep-alive");

      const reader = upstream.body!.getReader();
      const decoder = new TextDecoder();
      const state: StreamState = { started: false, blockIndex: 0, textBlockOpen: false, toolCalls: new Map(), inputTokens: 0, outputTokens: 0 };
      const model = body.model || currentTarget.model;

      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) {
            if (state.started) {
              if (state.textBlockOpen) res.write(makeSSE("content_block_stop", { type: "content_block_stop", index: state.blockIndex }));
              res.write(makeSSE("message_delta", { type: "message_delta", delta: { stop_reason: "end_turn" }, usage: { output_tokens: state.outputTokens } }));
              res.write(makeSSE("message_stop", { type: "message_stop" }));
            }
            break;
          }
          const text = decoder.decode(value, { stream: true });
          for (const line of text.split("\n")) {
            const trimmed = line.trim();
            if (!trimmed || trimmed === "data: [DONE]" || !trimmed.startsWith("data: ")) continue;
            try {
              const chunk = JSON.parse(trimmed.slice(6));
              const sseOutput = processStreamChunk(chunk, state, model);
              if (sseOutput) res.write(sseOutput);
            } catch { /* skip */ }
          }
        }
      } catch (err) {
        console.error("   ✗ [Proxy] Stream error:", err);
      }

      res.end();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Unknown proxy error";
      console.error(`   ✗ [Proxy] Error: ${msg}`);
      res.status(500).json({ type: "error", error: { type: "api_error", message: msg } });
    }
  });

  return router;
}
