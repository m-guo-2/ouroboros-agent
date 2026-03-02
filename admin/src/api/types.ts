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
  agentId: string
  userId: string
  sourceChannel: string
  sessionKey: string
  channelConversationId: string
  channelName: string
  workDir: string
  executionStatus: string
  createdAt: string
  updatedAt: string
}

export interface AgentSessionListItem extends AgentSession {
  messageCount: number
}

export interface MessageData {
  id: string
  sessionId: string
  role: string
  content: string
  messageType?: string
  channel?: string
  traceId?: string
  initiator?: string
  senderName?: string
  senderId?: string
  createdAt?: string
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
    executor: { type: "http" | "shell" | "script" | "internal"; url?: string; method?: string; command?: string; handler?: string }
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
  type: "thinking" | "tool_call" | "tool_result" | "content" | "error" | "llm_call"
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
  /** llm_call：每次 LLM 调用的轻量统计 */
  model?: string
  inputTokens?: number
  outputTokens?: number
  durationMs?: number
  stopReason?: string
  costUsd?: number
  /** 完整 LLM I/O 文件引用（用于按需加载原始请求/响应） */
  llmIORef?: string
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

export interface TraceListItem {
  id: string
  startedAt: number
}
