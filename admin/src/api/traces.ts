import { fetchApi } from "./client"
import type { ExecutionTrace, TraceListItem } from "./types"

export const tracesApi = {
  getById: (id: string) => fetchApi<ExecutionTrace>(`/traces/${id}`),
  getBySession: (sessionId: string) => fetchApi<TraceListItem[]>(`/traces?sessionId=${sessionId}`),
  getRecent: (limit = 50) => fetchApi<TraceListItem[]>(`/traces?limit=${limit}`),
  getRecentSummaries: (limit = 30) => fetchApi<TraceListItem[]>(`/traces/recent-summaries?limit=${limit}`),
  getLLMIO: (traceId: string, ref: string) => fetchApi<Record<string, unknown>>(`/traces/${traceId}/llm-io/${ref}`),
  listLLMIORefs: (traceId: string) => fetchApi<string[]>(`/traces/${traceId}/llm-io`),
}
