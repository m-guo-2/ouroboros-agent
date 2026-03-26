import { useQuery } from "@tanstack/react-query"
import { subagentJobsApi } from "@/api/subagent-jobs"
import type { SubagentJobDetail } from "@/api/types"

export function useSubagentJobDetail(jobId: string | null) {
  return useQuery<SubagentJobDetail | null>({
    queryKey: ["subagent-jobs", jobId],
    queryFn: async () => {
      if (!jobId) return null
      const res = await subagentJobsApi.getJobDetail(jobId)
      return res.data ?? null
    },
    enabled: !!jobId,
    staleTime: 10_000,
  })
}
