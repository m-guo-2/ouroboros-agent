import { fetchApi } from "./client"
import type { SubagentJobSummary, SubagentJobDetail } from "./types"

export const subagentJobsApi = {
  getSessionJobs: (sessionId: string) =>
    fetchApi<SubagentJobSummary[]>(`/agent-sessions/${sessionId}/subagent-jobs`),

  getJobDetail: (jobId: string) =>
    fetchApi<SubagentJobDetail>(`/subagent-jobs/${jobId}`),
}
