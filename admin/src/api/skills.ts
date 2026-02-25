import { fetchApi } from "./client"
import type { SkillListItem, SkillDetail, SkillManifest, SkillVersionSummary, SkillVersionDetail } from "./types"

export const skillsApi = {
  getAll: () => fetchApi<SkillListItem[]>("/skills"),
  getById: (name: string) => fetchApi<SkillDetail>(`/skills/${name}`),

  create: (name: string, manifest: Omit<SkillManifest, "version">, readme?: string) =>
    fetchApi<{ name: string; manifest: SkillManifest }>("/skills", {
      method: "POST",
      body: JSON.stringify({ name, manifest, readme }),
    }),

  update: (name: string, manifest?: Partial<SkillManifest>, readme?: string, changeSummary?: string) =>
    fetchApi<{ name: string; manifest: SkillManifest }>(`/skills/${name}`, {
      method: "PUT",
      body: JSON.stringify({ manifest, readme, changeSummary }),
    }),

  toggle: (name: string, enabled: boolean) =>
    fetchApi<{ name: string; enabled: boolean }>(`/skills/${name}/toggle`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),

  delete: (name: string) => fetchApi<void>(`/skills/${name}`, { method: "DELETE" }),

  getVersions: (name: string) => fetchApi<SkillVersionSummary[]>(`/skills/${name}/versions`),

  getVersion: (name: string, version: number) =>
    fetchApi<SkillVersionDetail>(`/skills/${name}/versions/${version}`),

  restoreVersion: (name: string, version: number) =>
    fetchApi<{ name: string; version: number; message: string }>(
      `/skills/${name}/versions/${version}/restore`,
      { method: "POST" },
    ),
}
