import { useQuery } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { DelayedTask } from "@/api/types"

export function useSessionDelayedTasks(sessionId: string | null, status?: string, enabled = true) {
  return useQuery<DelayedTask[]>({
    queryKey: ["sessions", sessionId, "delayed-tasks", status ?? "all"],
    queryFn: async () => {
      if (!sessionId) return []
      const res = await sessionsApi.getDelayedTasks(sessionId, status)
      return res.data ?? []
    },
    enabled: !!sessionId && enabled,
    staleTime: 30_000,
  })
}
