import { useQuery } from "@tanstack/react-query"
import { subagentJobsApi } from "@/api/subagent-jobs"
import type { SubagentJobSummary } from "@/api/types"

export function useSessionSubagentJobs(sessionId: string | null, enabled = true) {
  return useQuery<SubagentJobSummary[]>({
    queryKey: ["sessions", sessionId, "subagent-jobs"],
    queryFn: async () => {
      if (!sessionId) return []
      const res = await subagentJobsApi.getSessionJobs(sessionId)
      return res.data ?? []
    },
    enabled: !!sessionId && enabled,
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
}
