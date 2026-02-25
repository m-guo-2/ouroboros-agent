import { fetchApi } from "./client"
import type { AgentProfile } from "./types"

export const agentsApi = {
  getAll: () => fetchApi<AgentProfile[]>("/agents"),
  getActive: () => fetchApi<AgentProfile[]>("/agents/active"),
  getById: (id: string) => fetchApi<AgentProfile>(`/agents/${id}`),

  create: (data: {
    displayName: string
    systemPrompt?: string
    modelId?: string
    skills?: string[]
    channels?: Array<{ type: string; identifier: string }>
    avatarUrl?: string
  }) => fetchApi<AgentProfile>("/agents", { method: "POST", body: JSON.stringify(data) }),

  update: (id: string, data: Partial<Omit<AgentProfile, "id" | "createdAt" | "updatedAt">>) =>
    fetchApi<AgentProfile>(`/agents/${id}`, { method: "PUT", body: JSON.stringify(data) }),

  delete: (id: string) => fetchApi<void>(`/agents/${id}`, { method: "DELETE" }),
}
