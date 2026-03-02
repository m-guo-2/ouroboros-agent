import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { AgentSession, MessageData } from "@/api/types"

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
      const data = query.state.data
      return data?.executionStatus === "processing" ? 2000 : false
    },
  })
}

export function useSessionMessages(sessionId: string | undefined, opts?: { refetchInterval?: number | false }) {
  return useQuery<MessageData[]>({
    queryKey: ["sessions", sessionId, "messages"],
    queryFn: async () => {
      const res = await sessionsApi.getMessages(sessionId!)
      return res.data ?? []
    },
    enabled: !!sessionId,
    refetchInterval: opts?.refetchInterval ?? 5000,
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
