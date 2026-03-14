import { useMemo } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { AgentSessionListItem } from "@/api/types"

const PAGE_SIZE = 10

export function useMonitorSessions(filters?: { agentId?: string; channel?: string; limit?: number }) {
  const pageSize = filters?.limit ?? PAGE_SIZE

  const query = useInfiniteQuery<AgentSessionListItem[]>({
    queryKey: ["monitor", "sessions", filters],
    queryFn: async ({ pageParam }) => {
      const res = await sessionsApi.getAll({
        ...filters,
        limit: pageSize,
        before: pageParam as number | undefined,
      })
      return res.data ?? []
    },
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (lastPage) => {
      if (lastPage.length < pageSize) return undefined
      const last = lastPage[lastPage.length - 1]
      return last?.updatedAt ? new Date(last.updatedAt).getTime() : undefined
    },
  })

  const sessions = useMemo(
    () => query.data?.pages.flat() ?? [],
    [query.data],
  )

  return { ...query, sessions }
}
