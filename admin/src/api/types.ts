export interface AvailableModel {
  id: string
  name: string
  provider: string
  contextLength?: number
  description?: string
}

// ===== Agent Profiles =====

export interface SubagentModelConfig {
  provider: string
  model: string
}

export interface AgentProfile {
  id: string
  displayName: string
  systemPrompt?: string
  modelId?: string
  provider?: string
  model?: string
  sandboxTemplateId?: string
  subagentModels?: Record<string, SubagentModelConfig>
  skills?: string[]
  subagentSkills?: Record<string, string[]>
  hooks?: Hook[]
  channels?: Array<{ type: string; identifier: string }>
  isActive?: boolean
  avatarUrl?: string
  isDefault?: boolean
  createdAt?: number
  updatedAt?: number
}

// ===== Hooks =====

export interface HookAction {
  type: string
  skillId?: string
  scopeType?: string
  expiresAfterEvents?: number
}

export interface Hook {
  event: string
  actions: HookAction[]
}

export interface HookEventDef {
  name: string
  label: string
  description: string
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
  sandboxTemplateId: string
  executionStatus: string
  createdAt: number
  updatedAt: number
}

export interface AgentSessionListItem extends AgentSession {
  messageCount: number
}

export interface MessageData {
  id: number
  sessionId: string
  role: string
  content: string
  messageType?: string
  channel?: string
  channelMessageId?: string
  traceId?: string
  initiator?: string
  senderName?: string
  senderId?: string
  createdAt?: number
}

export interface MessageLifecycleEvent {
  id: number
  sessionId: string
  messageId?: number
  traceId?: string
  channelMessageId?: string
  stage: string
  status: "success" | "failed" | string
  outcome?: "replied" | "no_reply" | "send_failed" | string
  summary?: string
  payload?: Record<string, unknown>
  createdAt: number
}

// ===== Skills =====

export interface SandboxTemplate {
  id: string
  category: string
  display_name: string
  description: string
  runtime_commands: string[]
  python_packages: string[]
  system_binaries: string[]
  verification_hint: string
}

export interface SkillListItem {
  id: string
  name: string
  description: string
  enabled: boolean
  scripts?: string[]
  references?: string[]
}

export interface SkillDetail {
  id: string
  name: string
  description: string
  enabled: boolean
  readme: string
  scripts?: string[]
  references?: string[]
}

// ===== Settings =====

export interface SettingKeyDef {
  key: string
  label: string
  secret?: boolean
  placeholder?: string
  description?: string
  type?: "provider-select" | "model-select" | "select"
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
  type: "thinking" | "tool_call" | "tool_result" | "content" | "error" | "llm_call" | "absorb" | "compact" | "subagent_reentry" | "mode_change"
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
  /** absorb 事件：吸纳轮次 */
  absorbRound?: number
  /** absorb 事件：吸纳消息数 */
  absorbedCount?: number
  /** compact 事件：压缩前 token 数 */
  tokensBefore?: number
  /** compact 事件：压缩后 token 数 */
  tokensAfter?: number
  /** compact 事件：归档消息数 */
  archivedCount?: number
  /** tool_result (run_subagent_async)：关联的 subagent trace ID */
  subTraceId?: string
  /** mode_change 事件：切换前模式 */
  modeFrom?: string
  /** mode_change 事件：切换后模式 */
  modeTo?: string
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

// ===== Session Facts =====

export interface SessionFact {
  id: number
  sessionId: string
  fact: string
  category: string
  createdAt: number
}

// ===== Delayed Tasks =====

export interface DelayedTask {
  id: number
  sessionId: string
  agentId: string
  userId: string
  channel: string
  channelUserId: string
  channelConversationId: string
  task: string
  executeAt: number
  status: "pending" | "dispatched" | "cancelled"
  createdAt: number
  updatedAt: number
}

// ===== Compactions =====

export interface CompactionData {
  id: number
  sessionId: string
  summary: string
  archivedBeforeTime: number
  archivedMessageCount: number
  tokenCountBefore: number
  tokenCountAfter: number
  compactModel: string
  createdAt: number
}

// ===== Subagent Jobs =====

export interface SubagentJobSummary {
  id: string
  name: string
  profile: string
  status: "queued" | "running" | "completed" | "failed" | "canceled"
  subTraceId: string
  parentTraceId: string
  createdAt: number
  updatedAt: number
  impactCount: number
  task: string
}

export interface SubagentImpact {
  timestamp: number
  tool: string
  summary: string
  detail?: Record<string, unknown>
}

export interface SubagentJobDetail {
  id: string
  name: string
  profile: string
  task: string
  status: "queued" | "running" | "completed" | "failed" | "canceled"
  subTraceId: string
  parentTraceId: string
  sessionId: string
  createdAt: number
  updatedAt: number
  result?: string
  error?: string
  impacts?: SubagentImpact[]
  events: Array<Record<string, unknown>>
}

// ===== Personas & group assignments =====

export interface Persona {
  id: string
  agentId: string
  displayName: string
  systemPrompt?: string | null
  provider?: string | null
  model?: string | null
  sandboxTemplateId?: string | null
  skills?: string[] | null
  subagentModels?: Record<string, SubagentModelConfig> | null
  subagentSkills?: Record<string, string[]> | null
  groupCount: number
  createdAt: string
  updatedAt: string
}

export interface GroupAssignment {
  id: string
  agentId: string
  sessionKey: string
  groupName: string
  personaId?: string | null
  sandboxTemplateId?: string | null
  createdAt: string
  updatedAt: string
}

export interface UnconfiguredGroup {
  sessionKey: string
  channelName: string
  sourceChannel: string
  lastActive: number
}
