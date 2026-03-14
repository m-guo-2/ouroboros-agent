import { useMemo } from "react"
import { useQuery, useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { AgentSession, MessageData } from "@/api/types"

const MESSAGES_PAGE_SIZE = 10

export function useSessions(filters?: { agentId?: string; channel?: string; limit?: number }) {
  return useQuery({
    queryKey: ["sessions", filters],
    queryFn: async () => {
      const res = await sessionsApi.getAll(filters)
      return res.data ?? []
    },
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
  })
}

export function useSessionMessages(sessionId: string | undefined, opts?: { refetchInterval?: number | false; limit?: number }) {
  const pageSize = opts?.limit ?? MESSAGES_PAGE_SIZE

  const query = useInfiniteQuery<MessageData[]>({
    queryKey: ["sessions", sessionId, "messages"],
    queryFn: async ({ pageParam }) => {
      const res = await sessionsApi.getMessages(sessionId!, pageSize, pageParam as number | undefined)
      return res.data ?? []
    },
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (lastPage) => {
      if (lastPage.length < pageSize) return undefined
      const oldest = lastPage[0]
      return oldest?.createdAt ? Number(oldest.createdAt) : undefined
    },
    enabled: !!sessionId,
    refetchInterval: opts?.refetchInterval ?? false,
  })

  const messages = useMemo(
    () => query.data?.pages ? [...query.data.pages].reverse().flat() : [],
    [query.data],
  )

  return { ...query, messages }
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
