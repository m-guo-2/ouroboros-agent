import { fetchApi } from "./client"
import type { AgentProfile, HookEventDef } from "./types"

export const agentsApi = {
  getAll: () => fetchApi<AgentProfile[]>("/agents"),
  getActive: () => fetchApi<AgentProfile[]>("/agents/active"),
  getById: (id: string) => fetchApi<AgentProfile>(`/agents/${id}`),

  create: (data: {
    displayName: string
    systemPrompt?: string
    modelId?: string
    skills?: string[]
    subagentSkills?: Record<string, string[]>
    channels?: Array<{ type: string; identifier: string }>
    avatarUrl?: string
  }) => fetchApi<AgentProfile>("/agents", { method: "POST", body: JSON.stringify(data) }),

  update: (id: string, data: Partial<Omit<AgentProfile, "id" | "createdAt" | "updatedAt">>) =>
    fetchApi<AgentProfile>(`/agents/${id}`, { method: "PUT", body: JSON.stringify(data) }),

  delete: (id: string) => fetchApi<void>(`/agents/${id}`, { method: "DELETE" }),

  getFullPrompt: (id: string) =>
    fetchApi<{ fullPrompt: string }>(`/agents/${id}/full-prompt`),

  previewFullPrompt: (id: string, data: { systemPrompt: string; skills: string[] }) =>
    fetchApi<{ fullPrompt: string }>(`/agents/${id}/full-prompt`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getHookEvents: () => fetchApi<HookEventDef[]>("/hook-events"),
}
