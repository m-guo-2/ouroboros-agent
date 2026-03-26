import { useState, useMemo, useCallback, useEffect } from "react"
import { Activity, MessageSquare, Brain, Clock, GitBranch, PanelRight, RefreshCw } from "lucide-react"
import { useMonitorSessions } from "@/hooks/use-monitor"
import { useSession, useSessionMessages, useDeleteSession } from "@/hooks/use-sessions"
import { useMonitorSearchParams } from "@/hooks/use-monitor-search-params"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { tracesApi } from "@/api/traces"
import type { ExecutionTrace } from "@/api/types"
import { cn } from "@/lib/utils"
import { useSessionCompactions } from "./hooks/use-session-compactions"
import { useSessionFacts } from "./hooks/use-session-facts"
import { useSessionDelayedTasks } from "./hooks/use-session-delayed-tasks"
import { useSessionSubagentJobs } from "./hooks/use-session-subagent-jobs"
import { buildExchanges } from "./lib/build-timeline"
import { SessionList } from "./components/session-list"
import { DeleteSessionDialog } from "./components/delete-session-dialog"
import { ConversationTimeline } from "./components/conversation-timeline"
import { DecisionInspector } from "./components/decision-inspector"
import { SessionMemoryPanel } from "./components/session-memory-panel"
import { SessionDelayedTasksPanel } from "./components/session-delayed-tasks-panel"
import { SubagentJobsPanel } from "./components/subagent-jobs-panel"

export type MonitorTab = "conversation" | "memory" | "tasks" | "subagent"

const TABS: { id: MonitorTab; label: string; icon: typeof MessageSquare }[] = [
  { id: "conversation", label: "对话", icon: MessageSquare },
  { id: "memory", label: "记忆", icon: Brain },
  { id: "tasks", label: "定时任务", icon: Clock },
  { id: "subagent", label: "Subagent", icon: GitBranch },
]

export function MonitorPage() {
  const queryClient = useQueryClient()
  const urlState = useMonitorSearchParams()
  const [inspectorOpen, setInspectorOpen] = useState(true)
  const [searchQuery, setSearchQuery] = useState("")
  const [statusFilter, setStatusFilter] = useState("")
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null)

  const sessionFilters = useMemo(() => {
    const f: { status?: string; search?: string } = {}
    if (statusFilter) f.status = statusFilter
    if (searchQuery) f.search = searchQuery
    return f
  }, [statusFilter, searchQuery])

  const {
    sessions,
    isLoading,
    isFetching: isRefreshingSessions,
    hasNextPage: hasMoreSessions,
    fetchNextPage: fetchMoreSessions,
    isFetchingNextPage: isFetchingMoreSessions,
    error: sessionsError,
  } = useMonitorSessions(sessionFilters)
  const deleteSession = useDeleteSession()

  const effectiveSessionId = useMemo(() => {
    if (urlState.sessionId) {
      if (sessions && sessions.length > 0) {
        const exists = sessions.some(s => s.id === urlState.sessionId)
        if (exists) return urlState.sessionId
      } else if (isLoading) {
        return urlState.sessionId
      }
    }
    if (!sessions || sessions.length === 0) return null
    const processing = sessions.find((s) => s.executionStatus === "processing")
    return processing?.id ?? sessions[0].id
  }, [urlState.sessionId, sessions, isLoading])

  useEffect(() => {
    if (!isLoading && urlState.sessionId && sessions && sessions.length > 0) {
      const exists = sessions.some(s => s.id === urlState.sessionId)
      if (!exists) urlState.clearInvalidSession()
    }
  }, [isLoading, urlState.sessionId, sessions])

  const handleDeleteSession = useCallback((e: React.MouseEvent, sessionId: string) => {
    e.stopPropagation()
    setDeleteTargetId(sessionId)
  }, [])

  const confirmDeleteSession = useCallback(() => {
    if (!deleteTargetId) return
    deleteSession.mutate(deleteTargetId, {
      onSuccess: () => {
        if (effectiveSessionId === deleteTargetId) {
          urlState.setSessionId(null)
        }
      },
    })
    setDeleteTargetId(null)
  }, [deleteSession, deleteTargetId, effectiveSessionId, urlState])

  const handleSelectExchange = useCallback((idx: number) => {
    urlState.setExchangeIndex(idx)
    if (!inspectorOpen) setInspectorOpen(true)
  }, [inspectorOpen, urlState])

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
  } = useSessionMessages(effectiveSessionId ?? "", { isProcessing: !!isProcessing })

  const totalMessageCount = useMemo(() => {
    if (!effectiveSessionId || !sessions) return 0
    return sessions.find(s => s.id === effectiveSessionId)?.messageCount ?? messages.length
  }, [effectiveSessionId, sessions, messages.length])

  const { data: compactions = [] } = useSessionCompactions(effectiveSessionId)

  const { data: factsForTabCount } = useSessionFacts(effectiveSessionId, true)
  const { data: delayedTasksForTabCount } = useSessionDelayedTasks(effectiveSessionId, undefined, true)
  const { data: subagentJobsForTabCount } = useSessionSubagentJobs(effectiveSessionId, true)

  const tabCounts = useMemo(
    (): Record<MonitorTab, number> => ({
      conversation: 0,
      memory: factsForTabCount?.length ?? 0,
      tasks: delayedTasksForTabCount?.length ?? 0,
      subagent: subagentJobsForTabCount?.length ?? 0,
    }),
    [factsForTabCount, delayedTasksForTabCount, subagentJobsForTabCount]
  )

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
    if (urlState.exchangeIndex != null) {
      const ex = exchanges.find((exchange) => exchange.exchangeIndex === urlState.exchangeIndex)
      if (ex?.traceId) return urlState.exchangeIndex
    }
    return latestTraceExchangeIndex
  }, [urlState.exchangeIndex, latestTraceExchangeIndex, exchanges])

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
    staleTime: 10_000,
    refetchInterval: (q) => (q.state.data?.status === "running" ? 3000 : false),
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
      <SessionList
        sessions={sessions}
        isLoading={isLoading}
        selectedSessionId={effectiveSessionId}
        onSelectSession={(id) => { urlState.setSessionId(id); urlState.setTab("conversation") }}
        onDeleteSession={handleDeleteSession}
        onRefresh={handleRefreshSessions}
        isRefreshing={isRefreshingSessions}
        hasMore={!!hasMoreSessions}
        onLoadMore={() => void fetchMoreSessions()}
        isLoadingMore={isFetchingMoreSessions}
        onSearchChange={setSearchQuery}
        onStatusChange={setStatusFilter}
        error={sessionsError}
      />

      <div className="flex-1 bg-slate-50 flex flex-col min-w-0">
        {effectiveSessionId ? (
          <>
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
                {urlState.tab === "conversation" && (
                  <button
                    onClick={handleRefreshMessages}
                    disabled={isRefreshingMessages}
                    className="p-1.5 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600 disabled:opacity-40 transition-colors"
                    title="刷新对话"
                  >
                    <RefreshCw className={cn("h-3.5 w-3.5", isRefreshingMessages && "animate-spin")} />
                  </button>
                )}
                {!inspectorOpen && (
                  <button onClick={() => setInspectorOpen(true)}
                    className="p-1.5 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
                    <PanelRight className="h-4 w-4" />
                  </button>
                )}
              </div>
            </div>

            <div className="flex items-center gap-0.5 px-5 py-1.5 border-b border-slate-200 bg-white shrink-0">
              {TABS.map((tab) => {
                const Icon = tab.icon
                const count = tabCounts[tab.id]
                return (
                  <button
                    key={tab.id}
                    onClick={() => urlState.setTab(tab.id)}
                    className={cn(
                      "flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-colors",
                      urlState.tab === tab.id
                        ? "bg-slate-100 text-slate-900"
                        : "text-slate-500 hover:text-slate-700 hover:bg-slate-50"
                    )}
                  >
                    <Icon className="h-3.5 w-3.5" />
                    {tab.label}
                    {tab.id !== "conversation" && count > 0 && (
                      <span className="ml-1 px-1.5 py-0.5 rounded-full bg-slate-200 text-[10px] font-medium text-slate-600">
                        {count}
                      </span>
                    )}
                  </button>
                )
              })}
            </div>

            {urlState.tab === "conversation" && (
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
            )}
            {urlState.tab === "memory" && (
              <SessionMemoryPanel sessionId={effectiveSessionId} enabled={urlState.tab === "memory"} />
            )}
            {urlState.tab === "tasks" && (
              <SessionDelayedTasksPanel sessionId={effectiveSessionId} enabled={urlState.tab === "tasks"} />
            )}
            {urlState.tab === "subagent" && (
              <SubagentJobsPanel
                sessionId={effectiveSessionId}
                enabled={urlState.tab === "subagent"}
                onViewTrace={() => urlState.setTab("conversation")}
              />
            )}
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

      <DeleteSessionDialog
        open={!!deleteTargetId}
        onOpenChange={(open) => { if (!open) setDeleteTargetId(null) }}
        sessionName={deleteTargetId ? (sessions?.find(s => s.id === deleteTargetId)?.channelName || sessions?.find(s => s.id === deleteTargetId)?.title || deleteTargetId.slice(0, 10)) : ""}
        onConfirm={confirmDeleteSession}
      />
    </div>
  )
}
