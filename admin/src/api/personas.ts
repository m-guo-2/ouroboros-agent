import { fetchApi } from "./client"
import type { Persona, GroupAssignment, UnconfiguredGroup } from "./types"

export const personasApi = {
  list: (agentId: string) => fetchApi<Persona[]>(`/agents/${agentId}/personas`),
  get: (agentId: string, id: string) => fetchApi<Persona>(`/agents/${agentId}/personas/${id}`),
  create: (agentId: string, data: Partial<Persona>) =>
    fetchApi<Persona>(`/agents/${agentId}/personas`, { method: "POST", body: JSON.stringify(data) }),
  update: (agentId: string, id: string, data: Partial<Persona>) =>
    fetchApi<Persona>(`/agents/${agentId}/personas/${id}`, { method: "PUT", body: JSON.stringify(data) }),
  clone: (agentId: string, id: string, displayName: string) =>
    fetchApi<Persona>(`/agents/${agentId}/personas/${id}/clone`, {
      method: "POST",
      body: JSON.stringify({ displayName }),
    }),
  delete: (agentId: string, id: string) =>
    fetchApi<void>(`/agents/${agentId}/personas/${id}`, { method: "DELETE" }),
}

export const groupAssignmentsApi = {
  list: (agentId: string) => fetchApi<GroupAssignment[]>(`/agents/${agentId}/groups`),
  discover: (agentId: string) =>
    fetchApi<UnconfiguredGroup[]>(`/agents/${agentId}/groups/discover`),
  create: (agentId: string, data: { sessionKey: string; groupName: string; personaId?: string; sandboxTemplateId?: string | null }) =>
    fetchApi<GroupAssignment>(`/agents/${agentId}/groups`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  update: (agentId: string, id: string, data: { personaId?: string; groupName?: string; sandboxTemplateId?: string | null }) =>
    fetchApi<GroupAssignment>(`/agents/${agentId}/groups/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  delete: (agentId: string, id: string) =>
    fetchApi<void>(`/agents/${agentId}/groups/${id}`, { method: "DELETE" }),
  clearHistory: (agentId: string, id: string) =>
    fetchApi<{ cleared: boolean; sessionId: string; messageCount: number }>(`/agents/${agentId}/groups/${id}/clear-history`, { method: "POST" }),
}
