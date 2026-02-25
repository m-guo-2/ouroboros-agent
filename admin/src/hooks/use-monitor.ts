import { useQuery } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import { tracesApi } from "@/api/traces"
import type { AgentSessionListItem, TraceSummary } from "@/api/types"

/**
 * Monitor sessions list — polls faster when any session is processing
 */
export function useMonitorSessions(filters?: { agentId?: string; channel?: string; limit?: number }) {
  const query = useQuery<AgentSessionListItem[]>({
    queryKey: ["monitor", "sessions", filters],
    queryFn: async () => {
      const res = await sessionsApi.getAll({ ...filters, limit: filters?.limit ?? 50 })
      return res.data ?? []
    },
    refetchInterval: (query) => {
      // Poll faster if any session is processing
      const data = query.state.data
      const hasActive = data?.some((s) => s.executionStatus === "processing")
      return hasActive ? 3000 : 10000
    },
  })

  return query
}

/**
 * Active traces — only fetches when there are processing sessions
 */
export function useActiveTraces(enabled = true) {
  return useQuery<TraceSummary[]>({
    queryKey: ["traces", "active"],
    queryFn: async () => {
      const res = await tracesApi.getActive()
      return res.data ?? []
    },
    refetchInterval: 3000,
    enabled,
  })
}

export function useRecentTraces(limit = 30) {
  return useQuery<TraceSummary[]>({
    queryKey: ["traces", "recent", limit],
    queryFn: async () => {
      const res = await tracesApi.getRecentSummaries(limit)
      return res.data ?? []
    },
    refetchInterval: 8000,
  })
}
