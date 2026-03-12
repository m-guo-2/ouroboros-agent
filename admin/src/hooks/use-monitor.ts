import { useQuery } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { AgentSessionListItem } from "@/api/types"

export function useMonitorSessions(filters?: { agentId?: string; channel?: string; limit?: number }) {
  const query = useQuery<AgentSessionListItem[]>({
    queryKey: ["monitor", "sessions", filters],
    queryFn: async () => {
      const res = await sessionsApi.getAll({ ...filters, limit: filters?.limit ?? 50 })
      return res.data ?? []
    },
  })

  return query
}
