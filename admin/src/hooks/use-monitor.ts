import { useMemo } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { sessionsApi } from "@/api/sessions"
import type { AgentSessionListItem } from "@/api/types"

const PAGE_SIZE = 10

export function useMonitorSessions(filters?: { agentId?: string; channel?: string; limit?: number; status?: string; search?: string }) {
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
      return last?.updatedAt || undefined
    },
    refetchInterval: (q) => {
      const pages = q.state.data?.pages
      if (!pages?.length) return 30_000
      const flat = pages.flat()
      return flat.some((s) => s.executionStatus === "processing") ? 5_000 : 30_000
    },
    refetchIntervalInBackground: false,
  })

  const sessions = useMemo(
    () => query.data?.pages.flat() ?? [],
    [query.data],
  )

  return { ...query, sessions }
}
