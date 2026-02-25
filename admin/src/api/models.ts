import { fetchApi } from "./client"
import type { Model, AvailableModel } from "./types"

export const modelsApi = {
  getAll: () => fetchApi<Model[]>("/models"),
  getEnabled: () => fetchApi<Model[]>("/models/enabled"),
  getById: (id: string) => fetchApi<Model>(`/models/${id}`),

  update: (id: string, data: Partial<Model> & { apiKey?: string }) =>
    fetchApi<Model>(`/models/${id}`, { method: "PATCH", body: JSON.stringify(data) }),

  getAvailableModels: (id: string) =>
    fetchApi<AvailableModel[]>(`/models/${id}/available-models`),
}
