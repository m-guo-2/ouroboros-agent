import { fetchApi } from "./client"
import type { SkillListItem, SkillDetail } from "./types"

export interface BrowseSkillEntry {
  id: string
  name: string
  description: string
  exists: boolean
}

export interface BrowseResult {
  repo: string
  branch: string
  path: string
  skills: BrowseSkillEntry[]
}

export interface ImportResult {
  id: string
  ok: boolean
  error?: string
}

export interface ImportResponse {
  imported: number
  total: number
  results: ImportResult[]
}

export interface ImportParams {
  repo: string
  branch: string
  path: string
  skills: string[]
  overwrite?: boolean
}

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

  browseImport: (url: string) =>
    fetchApi<BrowseResult>("/skills/import/browse", {
      method: "POST",
      body: JSON.stringify({ url }),
    }),

  importSkills: (params: ImportParams) =>
    fetchApi<ImportResponse>("/skills/import", {
      method: "POST",
      body: JSON.stringify(params),
    }),
}
