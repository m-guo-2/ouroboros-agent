import { fetchApi } from "./client"
import type { AgentSession, AgentSessionListItem, MessageData, CompactionData } from "./types"

export const sessionsApi = {
  getAll: (filters?: { agentId?: string; channel?: string; userId?: string; limit?: number }) => {
    const params = new URLSearchParams()
    if (filters?.agentId) params.set("agentId", filters.agentId)
    if (filters?.channel) params.set("channel", filters.channel)
    if (filters?.userId) params.set("userId", filters.userId)
    if (filters?.limit) params.set("limit", String(filters.limit))
    const qs = params.toString()
    return fetchApi<AgentSessionListItem[]>(`/agent-sessions${qs ? `?${qs}` : ""}`)
  },

  getById: (id: string) => fetchApi<AgentSession>(`/agent-sessions/${id}`),

  getMessages: (id: string, limit = 200) =>
    fetchApi<MessageData[]>(`/agent-sessions/${id}/messages?limit=${limit}`),

  delete: (id: string) => fetchApi<void>(`/agent-sessions/${id}`, { method: "DELETE" }),

  getCompactions: (id: string) => fetchApi<CompactionData[]>(`/agent-sessions/${id}/compactions`),
}
