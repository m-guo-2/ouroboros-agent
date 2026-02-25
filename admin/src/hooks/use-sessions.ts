import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { AgentSession } from "@/api/types"

export function useSessions(filters?: { agentId?: string; channel?: string; limit?: number }) {
  return useQuery({
    queryKey: ["sessions", filters],
    queryFn: async () => {
      const res = await sessionsApi.getAll(filters)
      return res.data ?? []
    },
    refetchInterval: 10000,
  })
}

export function useSession(id: string | undefined) {
  return useQuery<AgentSession | undefined>({
    queryKey: ["sessions", id],
    queryFn: async () => {
      const res = await sessionsApi.getById(id!)
      return res.data
    },
    enabled: !!id,
    refetchInterval: (query) => {
      // Poll faster when the session is actively processing
      const data = query.state.data
      return data?.executionStatus === "processing" ? 2000 : false
    },
  })
}

export function useDeleteSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: sessionsApi.delete,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["sessions"] })
      qc.invalidateQueries({ queryKey: ["monitor", "sessions"] })
    },
  })
}
