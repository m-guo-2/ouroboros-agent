import { useState, useMemo, useCallback } from "react"
import { Activity, PanelRight, RefreshCw } from "lucide-react"
import { useMonitorSessions } from "@/hooks/use-monitor"
import { useSession, useSessionMessages, useDeleteSession } from "@/hooks/use-sessions"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { tracesApi } from "@/api/traces"
import type { ExecutionTrace } from "@/api/types"
import { cn } from "@/lib/utils"
import { useSessionCompactions } from "./hooks/use-session-compactions"
import { buildExchanges } from "./lib/build-timeline"
import { SessionList } from "./components/session-list"
import { ConversationTimeline } from "./components/conversation-timeline"
import { DecisionInspector } from "./components/decision-inspector"

export function MonitorPage() {
  const queryClient = useQueryClient()
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null)
  const [selectedExchangeIndex, setSelectedExchangeIndex] = useState<number | null>(null)
  const [search, setSearch] = useState("")
  const [inspectorOpen, setInspectorOpen] = useState(true)
  const {
    sessions,
    isLoading,
    isFetching: isRefreshingSessions,
    hasNextPage: hasMoreSessions,
    fetchNextPage: fetchMoreSessions,
    isFetchingNextPage: isFetchingMoreSessions,
  } = useMonitorSessions()
  const deleteSession = useDeleteSession()

  const effectiveSessionId = useMemo(() => {
    if (selectedSessionId) return selectedSessionId
    if (!sessions || sessions.length === 0) return null
    const processing = sessions.find((s) => s.executionStatus === "processing")
    return processing?.id ?? sessions[0].id
  }, [selectedSessionId, sessions])

  const handleDeleteSession = useCallback((e: React.MouseEvent, sessionId: string) => {
    e.stopPropagation()
    if (!confirm("确定删除此会话？将同时清除数据库记录、执行链路和 Agent 工作目录，不可恢复。")) return
    deleteSession.mutate(sessionId, {
      onSuccess: () => {
        if (effectiveSessionId === sessionId) {
          setSelectedSessionId(null)
          setSelectedExchangeIndex(null)
        }
      },
    })
  }, [deleteSession, effectiveSessionId])

  const handleSelectExchange = useCallback((idx: number) => {
    setSelectedExchangeIndex(idx)
    if (!inspectorOpen) setInspectorOpen(true)
  }, [inspectorOpen])

  // Session data
  const {
    data: session,
    isFetching: isFetchingSession,
  } = useSession(effectiveSessionId ?? "")
  const isProcessing = session?.executionStatus === "processing"
  const {
    messages,
    isLoading: isLoadingMessages,
    isFetching: isFetchingMessages,
    hasNextPage: hasMoreMessages,
    fetchNextPage: fetchMoreMessages,
    isFetchingNextPage: isFetchingMoreMessages,
  } = useSessionMessages(effectiveSessionId ?? "")

  const totalMessageCount = useMemo(() => {
    if (!effectiveSessionId || !sessions) return 0
    return sessions.find(s => s.id === effectiveSessionId)?.messageCount ?? messages.length
  }, [effectiveSessionId, sessions, messages.length])

  // Compactions
  const { data: compactions = [] } = useSessionCompactions(effectiveSessionId)

  // Build exchanges
  const exchanges = useMemo(() => {
    if (messages.length === 0) return []
    return buildExchanges(messages)
  }, [messages])

  const activeTraceId = useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].traceId) return messages[i].traceId
    }
    return undefined
  }, [messages])

  const latestTraceExchangeIndex = useMemo(() => {
    for (let i = exchanges.length - 1; i >= 0; i--) {
      if (exchanges[i].traceId) return exchanges[i].exchangeIndex
    }
    return null
  }, [exchanges])

  const effectiveExchangeIndex = useMemo(() => {
    if (selectedExchangeIndex != null) {
      const selectedExchange = exchanges.find((exchange) => exchange.exchangeIndex === selectedExchangeIndex)
      if (selectedExchange?.traceId) return selectedExchangeIndex
    }
    return latestTraceExchangeIndex
  }, [selectedExchangeIndex, latestTraceExchangeIndex, exchanges])

  const selectedExchange = useMemo(() => {
    if (effectiveExchangeIndex == null) return null
    return exchanges.find((exchange) => exchange.exchangeIndex === effectiveExchangeIndex) ?? null
  }, [effectiveExchangeIndex, exchanges])

  const selectedTraceId = selectedExchange?.traceId

  const {
    data: selectedTrace = null,
    refetch: refetchSelectedTrace,
    isFetching: isFetchingSelectedTrace,
  } = useQuery<ExecutionTrace | null>({
    queryKey: ["traces", selectedTraceId],
    queryFn: async () => {
      if (!selectedTraceId) return null
      const response = await tracesApi.getById(selectedTraceId)
      return response.data ?? null
    },
    enabled: !!selectedTraceId,
    staleTime: Infinity,
  })

  const handleRefreshSessions = useCallback(() => {
    void queryClient.resetQueries({ queryKey: ["monitor", "sessions"] })
  }, [queryClient])

  const handleRefreshMessages = useCallback(() => {
    if (!effectiveSessionId) return
    void queryClient.resetQueries({ queryKey: ["sessions", effectiveSessionId, "messages"] })
    void queryClient.invalidateQueries({ queryKey: ["sessions", effectiveSessionId] })
  }, [queryClient, effectiveSessionId])

  const handleRefreshTrace = useCallback(() => {
    if (!selectedTraceId) return
    void refetchSelectedTrace()
  }, [selectedTraceId, refetchSelectedTrace])

  const isRefreshingMessages = isFetchingSession || isFetchingMessages

  return (
    <div className="flex h-full">
      {/* Left: Session list */}
      <SessionList
        sessions={sessions}
        isLoading={isLoading}
        search={search}
        onSearchChange={setSearch}
        selectedSessionId={effectiveSessionId}
        onSelectSession={(id) => { setSelectedSessionId(id); setSelectedExchangeIndex(null) }}
        onDeleteSession={handleDeleteSession}
        onRefresh={handleRefreshSessions}
        isRefreshing={isRefreshingSessions}
        hasMore={!!hasMoreSessions}
        onLoadMore={() => void fetchMoreSessions()}
        isLoadingMore={isFetchingMoreSessions}
      />

      {/* Center: Conversation timeline */}
      <div className="flex-1 bg-slate-50 flex flex-col min-w-0">
        {effectiveSessionId ? (
          <>
            {/* Session header */}
            <div className="flex items-center justify-between px-5 py-3 border-b border-slate-200 bg-white shrink-0">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <h2 className="text-sm font-semibold text-slate-900 truncate">
                    {session?.channelName || session?.title || `会话 ${effectiveSessionId.slice(0, 8)}`}
                  </h2>
                  {isProcessing && <span className="h-2 w-2 rounded-full bg-green-500 animate-live-pulse" />}
                </div>
                <p className="text-xs text-slate-400 mt-0.5">
                  {totalMessageCount} 条消息
                  {messages.length < totalMessageCount && ` (已加载 ${messages.length})`}
                  {" · "}{exchanges.length} 次交互
                  {compactions.length > 0 && ` · ${compactions.length} 次压缩`}
                </p>
              </div>
              <div className="flex items-center gap-1.5 shrink-0">
                <button
                  onClick={handleRefreshMessages}
                  disabled={isRefreshingMessages}
                  className="p-1.5 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600 disabled:opacity-40 transition-colors"
                  title="刷新对话"
                >
                  <RefreshCw className={cn("h-3.5 w-3.5", isRefreshingMessages && "animate-spin")} />
                </button>
                {!inspectorOpen && (
                  <button onClick={() => setInspectorOpen(true)}
                    className="p-1.5 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
                    <PanelRight className="h-4 w-4" />
                  </button>
                )}
              </div>
            </div>

            <ConversationTimeline
              exchanges={exchanges}
              compactions={compactions}
              isProcessing={!!isProcessing}
              activeTraceId={activeTraceId}
              selectedTrace={selectedTrace}
              selectedExchangeIndex={effectiveExchangeIndex}
              onSelectExchange={handleSelectExchange}
              isLoadingMessages={isLoadingMessages}
              hasMoreMessages={!!hasMoreMessages}
              onLoadMoreMessages={() => void fetchMoreMessages()}
              isLoadingMoreMessages={isFetchingMoreMessages}
            />
          </>
        ) : (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-slate-100 mb-4">
              <Activity className="h-7 w-7 text-slate-400" />
            </div>
            <h3 className="text-sm font-medium text-slate-900">选择一个会话</h3>
            <p className="text-sm text-slate-500 mt-1 max-w-xs">
              从左侧选择一个会话，查看每条消息的完整处理过程
            </p>
          </div>
        )}
      </div>

      {/* Right: Decision inspector */}
      {inspectorOpen && effectiveSessionId && (
        <div className="w-[420px] shrink-0 border-l border-slate-200 bg-white">
          <DecisionInspector
            key={selectedTrace?.id ?? "empty-trace"}
            trace={selectedTrace}
            isSessionProcessing={isProcessing}
            onCollapse={() => setInspectorOpen(false)}
            onRefreshTrace={handleRefreshTrace}
            isRefreshingTrace={isFetchingSelectedTrace}
          />
        </div>
      )}
    </div>
  )
}
