import { fetchApi } from "./client"
import type { ExecutionTrace, MessageLifecycleEvent } from "./types"

export interface MonitorSummary {
  requestCount: number
  executionCount: number
  repliedCount: number
  noReplyCount: number
  executionFailedCount: number
  sendFailedCount: number
  runningCount: number
  replyRate: number
  avgDurationMs: number
  p95DurationMs: number
  inputTokens: number
  outputTokens: number
  totalCostUsd: number
  costCoverage: number
}

export interface MonitorTrendPoint {
  timestamp: number
  requestCount: number
  repliedCount: number
  failedCount: number
  avgDurationMs: number
  totalCostUsd: number
}

export interface MonitorOverview {
  summary: MonitorSummary
  trend: MonitorTrendPoint[]
  failureBreakdown: Array<{ stage: string; count: number }>
}

export interface ExecutionItem {
  id: string
  sessionId: string
  agentId: string
  userId: string
  channel: string
  sandboxTemplateId: string
  provider: string
  model: string
  status: "running" | "completed" | "failed"
  deliveryStatus: "not_started" | "sending" | "sent" | "failed"
  failureStage?: string
  errorMessage?: string
  requestCount: number
  llmCallCount: number
  toolCallCount: number
  toolFailureCount: number
  inputTokens: number
  outputTokens: number
  totalCostUsd: number
  costKnown: boolean
  startedAt: number
  completedAt: number
  repliedAt: number
  durationMs: number
  llmDurationMs: number
  toolDurationMs: number
  requestSummary: string
  outcome: "" | "replied" | "no_reply" | "send_failed" | "execution_failed"
}

export interface ExecutionDetail {
  execution: ExecutionItem
  requests: Array<{ id: number; content: string; status: string; outcome: string; createdAt: number }>
  lifecycle: MessageLifecycleEvent[]
  trace: ExecutionTrace | null
}

export interface MonitorFilters {
  from: number
  to: number
  agentId?: string
  channel?: string
  status?: string
  outcome?: string
  search?: string
  limit?: number
}

function query(filters: MonitorFilters) {
  const params = new URLSearchParams({ from: String(filters.from), to: String(filters.to) })
  if (filters.agentId) params.set("agentId", filters.agentId)
  if (filters.channel) params.set("channel", filters.channel)
  if (filters.status) params.set("status", filters.status)
  if (filters.outcome) params.set("outcome", filters.outcome)
  if (filters.search) params.set("search", filters.search)
  if (filters.limit) params.set("limit", String(filters.limit))
  return params.toString()
}

export const monitorApi = {
  overview: (filters: MonitorFilters) => fetchApi<MonitorOverview>(`/monitor/overview?${query(filters)}`),
  executions: (filters: MonitorFilters) => fetchApi<ExecutionItem[]>(`/monitor/executions?${query(filters)}`),
  execution: (id: string) => fetchApi<ExecutionDetail>(`/monitor/executions/${encodeURIComponent(id)}`),
}
