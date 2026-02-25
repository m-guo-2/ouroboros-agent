// ===== Models =====

export interface Model {
  id: string
  name: string
  provider: "claude" | "openai" | "kimi" | "glm"
  enabled: boolean
  configured: boolean
  model: string
  maxTokens: number
  temperature: number
  baseUrl?: string
  hasApiKey?: boolean
}

export interface AvailableModel {
  id: string
  name: string
  provider: string
  contextLength?: number
  description?: string
}

// ===== Agent Profiles =====

export interface AgentProfile {
  id: string
  displayName: string
  systemPrompt?: string
  modelId?: string
  provider?: string   // 直接指定 LLM 提供商
  model?: string      // 直接指定模型 ID
  skills?: string[]
  channels?: Array<{ type: string; identifier: string }>
  isActive?: boolean
  avatarUrl?: string
  isDefault?: boolean
  createdAt?: string
  updatedAt?: string
}

// ===== Sessions =====

export interface AgentSession {
  id: string
  title: string
  sdkSessionId?: string
  userId?: string
  agentId?: string
  agentDisplayName?: string
  sourceChannel?: string
  executionStatus?: string
  channelName?: string
  messages: AgentMessage[]
  createdAt?: string
  updatedAt?: string
}

export interface AgentSessionListItem {
  id: string
  title: string
  agentId?: string
  agentDisplayName?: string
  sourceChannel?: string
  executionStatus?: "idle" | "processing" | "completed" | "interrupted" | string
  channelName?: string
  messageCount: number
  createdAt?: string
  updatedAt?: string
}

export interface AgentMessage {
  role: "user" | "assistant" | "system"
  content: string
  toolCalls?: Array<{
    id: string
    tool: string
    input: unknown
    result?: unknown
    status: "pending" | "running" | "success" | "error"
  }>
  timestamp?: string
  traceId?: string
  initiator?: "user" | "agent" | "system"
  status?: "sending" | "sent" | "failed"
}

// ===== Skills =====

export interface SkillManifest {
  name: string
  description: string
  version: number
  type: "knowledge" | "action" | "hybrid"
  enabled: boolean
  triggers?: string[]
  tools?: Array<{
    name: string
    description: string
    inputSchema: { type: "object"; properties: Record<string, unknown>; required?: string[] }
    executor: { type: "http" | "script" | "internal"; url?: string; method?: string; command?: string; handler?: string }
  }>
}

export interface SkillListItem {
  name: string
  description: string
  version: number
  type: string
  enabled: boolean
  triggers: string[]
  toolCount: number
  tools: string[]
}

export interface SkillDetail {
  name: string
  manifest: SkillManifest
  readme: string
}

export interface SkillVersionSummary {
  version: number
  changeSummary: string
  createdAt: string
}

export interface SkillVersionDetail {
  version: number
  name: string
  description: string
  type: "knowledge" | "action" | "hybrid"
  triggers: string[]
  tools: SkillManifest["tools"]
  readme: string
  changeSummary: string
  createdAt: string
}

// ===== Settings =====

export interface SettingKeyDef {
  key: string
  label: string
  secret?: boolean
  placeholder?: string
  description?: string
  type?: "provider-select" | "model-select"
  options?: Array<{ value: string; label: string }>
  providerKey?: string  // model-select 关联的 provider 配置键
}

export interface SettingGroup {
  label: string
  keys: SettingKeyDef[]
}

// ===== Services =====

export interface ServiceInfo {
  name: string
  label: string
  description: string
  defaultPort: number
  status: "stopped" | "running" | "starting" | "error"
  pid?: number
  startedAt?: number
  error?: string
  externalProcess?: boolean
}

// ===== Traces =====

export interface ExecutionStep {
  index: number
  /** ReAct 迭代轮次（从 1 开始；system 步骤可能为 0） */
  iteration: number
  timestamp: number
  type: "thinking" | "tool_call" | "tool_result" | "content" | "error" | "model_io"
  thinking?: string
  /** 来源：model = 模型推理, system = 系统状态日志（加载配置/Skills 等） */
  source?: "model" | "system"
  toolCallId?: string
  toolName?: string
  toolInput?: unknown
  toolResult?: unknown
  toolDuration?: number
  toolSuccess?: boolean
  content?: string
  error?: string
  /** 每次 LLM 调用的完整输入/输出摘要 */
  modelInput?: unknown
  modelOutput?: unknown
}

export interface ExecutionTrace {
  id: string
  sessionId: string
  agentId?: string
  userId?: string
  channel?: string
  status: "running" | "completed" | "error"
  startedAt: number
  completedAt?: number
  inputTokens: number
  outputTokens: number
  totalCostUsd: number
  steps: ExecutionStep[]
}

export interface TraceSummary extends Omit<ExecutionTrace, "steps"> {
  thinkingCount: number
  toolCallCount: number
  toolErrorCount: number
  toolNames: string[]
  lastThinking?: string
  lastError?: string
}

// ===== Logs =====

export interface LogEntry {
  ts: string
  trace: string
  service: string
  op: string
  summary: string
  status?: string
  span?: string
  meta?: Record<string, unknown>
  data?: Record<string, unknown>
  error?: string
  _level?: string
}
