import { fetchApi } from "./client"
import type { SkillListItem, SkillDetail } from "./types"

export const skillsApi = {
  getAll: () => fetchApi<SkillListItem[]>("/skills"),
  getById: (id: string) => fetchApi<SkillDetail>(`/skills/${id}`),

  create: (data: Record<string, unknown>) =>
    fetchApi<SkillDetail>("/skills", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  update: (id: string, data: Record<string, unknown>) =>
    fetchApi<SkillDetail>(`/skills/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  toggle: (id: string, enabled: boolean) =>
    fetchApi<SkillDetail>(`/skills/${id}`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),

  delete: (id: string) => fetchApi<void>(`/skills/${id}`, { method: "DELETE" }),

  refresh: () => fetchApi<{ refreshed: number }>("/skills/refresh", { method: "POST" }),
}
