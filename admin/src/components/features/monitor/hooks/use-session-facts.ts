import { useQuery } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { SessionFact } from "@/api/types"

export function useSessionFacts(sessionId: string | null, enabled = true) {
  return useQuery<SessionFact[]>({
    queryKey: ["sessions", sessionId, "facts"],
    queryFn: async () => {
      if (!sessionId) return []
      const res = await sessionsApi.getFacts(sessionId)
      return res.data ?? []
    },
    enabled: !!sessionId && enabled,
    staleTime: 30_000,
  })
}
