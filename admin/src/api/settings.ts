import { fetchApi } from "./client"
import type { SettingGroup, AvailableModel } from "./types"

export const settingsApi = {
  getAll: () =>
    fetchApi<Record<string, string>>("/settings").then((res) =>
      res as unknown as { success: boolean; data: Record<string, string>; groups: Record<string, SettingGroup> }
    ),

  batchUpdate: (settings: Record<string, string>) =>
    fetchApi<{ message: string }>("/settings", { method: "PUT", body: JSON.stringify(settings) }),

  update: (key: string, value: string) =>
    fetchApi<{ key: string; value: string }>(`/settings/${key}`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    }),

  /** 查询指定 LLM 提供商的可用模型列表 */
  getProviderModels: (provider: string) =>
    fetchApi<AvailableModel[]>(`/settings/provider-models?provider=${encodeURIComponent(provider)}`),
}
