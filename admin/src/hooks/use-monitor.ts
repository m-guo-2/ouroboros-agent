import { useQuery } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import { tracesApi } from "@/api/traces"
import type { AgentSessionListItem, TraceListItem } from "@/api/types"

export function useMonitorSessions(filters?: { agentId?: string; channel?: string; limit?: number }) {
  const query = useQuery<AgentSessionListItem[]>({
    queryKey: ["monitor", "sessions", filters],
    queryFn: async () => {
      const res = await sessionsApi.getAll({ ...filters, limit: filters?.limit ?? 50 })
      return res.data ?? []
    },
    refetchInterval: (query) => {
      const data = query.state.data
      const hasActive = data?.some((s) => s.executionStatus === "processing")
      return hasActive ? 3000 : 10000
    },
  })

  return query
}

export function useRecentTraces(limit = 30) {
  return useQuery<TraceListItem[]>({
    queryKey: ["traces", "recent", limit],
    queryFn: async () => {
      const res = await tracesApi.getRecentSummaries(limit)
      return res.data ?? []
    },
    refetchInterval: 8000,
  })
}
