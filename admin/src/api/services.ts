import { fetchApi } from "./client"
import type { ServiceInfo } from "./types"

export const servicesApi = {
  getAll: () => fetchApi<ServiceInfo[]>("/services"),
  start: (name: string) => fetchApi<{ success: boolean; error?: string }>(`/services/${name}/start`, { method: "POST" }),
  stop: (name: string) => fetchApi<{ success: boolean; error?: string }>(`/services/${name}/stop`, { method: "POST" }),
  restart: (name: string) => fetchApi<{ success: boolean; error?: string }>(`/services/${name}/restart`, { method: "POST" }),
}
