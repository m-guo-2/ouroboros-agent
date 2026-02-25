import { fetchApi } from "./client"
import type { LogEntry } from "./types"

export const logsApi = {
  getByTrace: (traceId: string) => fetchApi<LogEntry[]>(`/logs/trace/${traceId}`),
  getBySpan: (spanId: string) => fetchApi<LogEntry[]>(`/logs/span/${spanId}`),

  getRecent: (options?: { level?: string; op?: string; limit?: number }) => {
    const params = new URLSearchParams()
    if (options?.level) params.set("level", options.level)
    if (options?.op) params.set("op", options.op)
    if (options?.limit) params.set("limit", String(options.limit))
    const qs = params.toString()
    return fetchApi<LogEntry[]>(`/logs/recent${qs ? `?${qs}` : ""}`)
  },
}
