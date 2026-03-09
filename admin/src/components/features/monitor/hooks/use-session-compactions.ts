import { useQuery } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { CompactionData } from "@/api/types"

export function useSessionCompactions(sessionId: string | null) {
  return useQuery<CompactionData[]>({
    queryKey: ["sessions", sessionId, "compactions"],
    queryFn: async () => {
      if (!sessionId) return []
      const res = await sessionsApi.getCompactions(sessionId)
      return res.data ?? []
    },
    enabled: !!sessionId,
    staleTime: 30_000,
  })
}
