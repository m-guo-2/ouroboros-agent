import { fetchApi } from "./client"
import type { ExecutionTrace, TraceSummary } from "./types"

export const tracesApi = {
  getById: (id: string) => fetchApi<ExecutionTrace>(`/traces/${id}`),
  getBySession: (sessionId: string) => fetchApi<Omit<ExecutionTrace, "steps">[]>(`/traces?sessionId=${sessionId}`),
  getRecent: (limit = 50) => fetchApi<Omit<ExecutionTrace, "steps">[]>(`/traces?limit=${limit}`),
  getActive: () => fetchApi<TraceSummary[]>("/traces/active"),
  getRecentSummaries: (limit = 30) => fetchApi<TraceSummary[]>(`/traces/recent-summaries?limit=${limit}`),
}
