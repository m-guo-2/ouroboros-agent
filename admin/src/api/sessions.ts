import { fetchApi } from "./client"
import type { AgentSession, AgentSessionListItem, MessageData, CompactionData, SessionFact, DelayedTask } from "./types"

export const sessionsApi = {
  getAll: (filters?: { agentId?: string; channel?: string; userId?: string; limit?: number; before?: number; status?: string; search?: string }) => {
    const params = new URLSearchParams()
    if (filters?.agentId) params.set("agentId", filters.agentId)
    if (filters?.channel) params.set("channel", filters.channel)
    if (filters?.userId) params.set("userId", filters.userId)
    if (filters?.limit) params.set("limit", String(filters.limit))
    if (filters?.before) params.set("before", String(filters.before))
    if (filters?.status) params.set("status", filters.status)
    if (filters?.search) params.set("search", filters.search)
    const qs = params.toString()
    return fetchApi<AgentSessionListItem[]>(`/agent-sessions${qs ? `?${qs}` : ""}`)
  },

  getById: (id: string) => fetchApi<AgentSession>(`/agent-sessions/${id}`),

  getMessages: (id: string, limit = 50, before?: number) => {
    const params = new URLSearchParams({ limit: String(limit) })
    if (before) params.set("before", String(before))
    return fetchApi<MessageData[]>(`/agent-sessions/${id}/messages?${params}`)
  },

  delete: (id: string) => fetchApi<void>(`/agent-sessions/${id}`, { method: "DELETE" }),

  getCompactions: (id: string) => fetchApi<CompactionData[]>(`/agent-sessions/${id}/compactions`),

  getFacts: (id: string) => fetchApi<SessionFact[]>(`/agent-sessions/${id}/facts`),

  getDelayedTasks: (id: string, status?: string) => {
    const params = new URLSearchParams()
    if (status && status !== "all") params.set("status", status)
    const qs = params.toString()
    return fetchApi<DelayedTask[]>(`/agent-sessions/${id}/delayed-tasks${qs ? `?${qs}` : ""}`)
  },
}
