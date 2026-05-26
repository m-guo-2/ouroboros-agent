import { useState, useMemo, useCallback, useEffect } from "react"
import { Activity, MessageSquare, Brain, Clock, GitBranch, PanelRight, RefreshCw, Radio, Inbox, CheckCircle2, AlertTriangle, Layers3 } from "lucide-react"
import { useMonitorSessions } from "@/hooks/use-monitor"
import { useSession, useSessionMessages, useDeleteSession, useSessionLifecycleEvents } from "@/hooks/use-sessions"
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
  const { data: lifecycleEvents = [] } = useSessionLifecycleEvents(effectiveSessionId ?? undefined, { isProcessing: !!isProcessing })

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
  const selectedMessageId = selectedExchange?.userMessage.id

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
  const lifecycleEventCount = lifecycleEvents.length
  const lifecycleByMessage = useMemo(() => {
    const map = new Map<number, typeof lifecycleEvents>()
    for (const event of lifecycleEvents) {
      if (!event.messageId) continue
      const items = map.get(event.messageId) ?? []
      items.push(event)
      map.set(event.messageId, items)
    }
    return map
  }, [lifecycleEvents])
  const outcomeStats = useMemo(() => {
    let replied = 0
    let noReply = 0
    let failed = 0
    let noLifecycle = 0
    for (const exchange of exchanges) {
      const events = exchange.userMessage.id ? lifecycleByMessage.get(exchange.userMessage.id) ?? [] : []
      const completed = [...events].reverse().find((event) => event.stage === "processed_completed")
      if (events.length === 0) noLifecycle += 1
      if (events.some((event) => event.status === "failed") || completed?.outcome === "send_failed") failed += 1
      else if (completed?.outcome === "no_reply") noReply += 1
      else if (completed?.outcome === "replied" || exchange.assistantMessage) replied += 1
    }
    return { replied, noReply, failed, noLifecycle }
  }, [exchanges, lifecycleByMessage])
  const currentOutcome = useMemo(() => {
    const completed = [...lifecycleEvents].reverse().find((event) => event.stage === "processed_completed")
    if (!completed?.outcome) return null
    if (completed.outcome === "replied") return "最近已回复"
    if (completed.outcome === "no_reply") return "最近未回复"
    if (completed.outcome === "send_failed") return "最近发送失败"
    return completed.outcome
  }, [lifecycleEvents])

  return (
    <div className="flex h-full bg-[#f6f8fb]">
      <SessionList
        sessions={sessions}
        isLoading={isLoading}
        selectedSessionId={effectiveSessionId}
        onSelectSession={(id) => urlState.selectSession(id)}
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

      <div className="flex-1 flex flex-col min-w-0">
        {effectiveSessionId ? (
          <>
            <div className="shrink-0 border-b border-slate-200 bg-white">
              <div className="bg-slate-950 px-6 py-5 text-white">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-[12px] font-medium text-slate-400">
                      <Radio className="h-3.5 w-3.5 text-cyan-300" />
                      Monitor Command Center
                      {isProcessing && <span className="ml-1 inline-flex items-center gap-1 rounded-full bg-emerald-400/15 px-2 py-0.5 text-emerald-200"><span className="h-1.5 w-1.5 rounded-full bg-emerald-300 animate-live-pulse" />处理中</span>}
                    </div>
                    <h2 className="mt-2 truncate text-xl font-semibold tracking-normal">
                      {session?.channelName || session?.title || `会话 ${effectiveSessionId.slice(0, 8)}`}
                    </h2>
                    <p className="mt-1 truncate text-[12px] text-slate-400">
                      {effectiveSessionId} · {session?.sourceChannel || "unknown"} · {session?.executionStatus || "unknown"}
                    </p>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    {urlState.tab === "conversation" && (
                      <button
                        onClick={handleRefreshMessages}
                        disabled={isRefreshingMessages}
                        className="inline-flex h-8 w-8 items-center justify-center rounded-md bg-white/10 text-slate-200 transition-colors hover:bg-white/15 disabled:opacity-40"
                        title="刷新对话"
                      >
                        <RefreshCw className={cn("h-3.5 w-3.5", isRefreshingMessages && "animate-spin")} />
                      </button>
                    )}
                    {!inspectorOpen && (
                      <button
                        onClick={() => setInspectorOpen(true)}
                        className="inline-flex h-8 w-8 items-center justify-center rounded-md bg-white/10 text-slate-200 transition-colors hover:bg-white/15"
                        title="打开处理详情"
                      >
                        <PanelRight className="h-4 w-4" />
                      </button>
                    )}
                  </div>
                </div>
                <div className="mt-5 grid grid-cols-2 gap-3 xl:grid-cols-4">
                  <CommandMetric icon={Inbox} label="收到消息" value={String(totalMessageCount)} detail={messages.length < totalMessageCount ? `已加载 ${messages.length}` : `${exchanges.length} 次交互`} tone="cyan" />
                  <CommandMetric icon={CheckCircle2} label="已回复" value={String(outcomeStats.replied)} detail={currentOutcome || "按当前加载消息统计"} tone="green" />
                  <CommandMetric icon={AlertTriangle} label="未回复/异常" value={String(outcomeStats.noReply + outcomeStats.failed)} detail={`${outcomeStats.noReply} 未回复 · ${outcomeStats.failed} 失败`} tone="amber" />
                  <CommandMetric icon={Layers3} label="消息流覆盖" value={String(lifecycleEventCount)} detail={`${outcomeStats.noLifecycle} 条无新版消息流`} tone={lifecycleEventCount > 0 ? "violet" : "muted"} />
                </div>
              </div>
            </div>

            <div className="flex items-center gap-1 border-b border-slate-200 bg-white px-6 py-2 shrink-0">
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
              <div className={cn(
                "grid min-h-0 flex-1 gap-4 bg-[#eef2f6] p-4",
                inspectorOpen ? "grid-cols-1 xl:grid-cols-[minmax(500px,0.98fr)_minmax(520px,1.02fr)]" : "grid-cols-1"
              )}>
                <div className="min-h-0 overflow-hidden rounded-xl border border-slate-300 bg-white shadow-sm">
                  <ConversationTimeline
                    exchanges={exchanges}
                    compactions={compactions}
                    isProcessing={!!isProcessing}
                    activeTraceId={activeTraceId}
                    selectedTrace={selectedTrace}
                    lifecycleEvents={lifecycleEvents}
                    selectedExchangeIndex={effectiveExchangeIndex}
                    onSelectExchange={handleSelectExchange}
                    isLoadingMessages={isLoadingMessages}
                    hasMoreMessages={!!hasMoreMessages}
                    onLoadMoreMessages={() => void fetchMoreMessages()}
                    isLoadingMoreMessages={isFetchingMoreMessages}
                  />
                </div>
                {inspectorOpen && (
                  <div className="min-h-0 overflow-hidden rounded-xl border border-slate-300 bg-white shadow-sm">
                    <DecisionInspector
                      key={selectedTrace?.id ?? "empty-trace"}
                      trace={selectedTrace}
                      lifecycleEvents={lifecycleEvents}
                      selectedMessageId={selectedMessageId}
                      isSessionProcessing={isProcessing}
                      onCollapse={() => setInspectorOpen(false)}
                      onRefreshTrace={handleRefreshTrace}
                      isRefreshingTrace={isFetchingSelectedTrace}
                    />
                  </div>
                )}
              </div>
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

      <DeleteSessionDialog
        open={!!deleteTargetId}
        onOpenChange={(open) => { if (!open) setDeleteTargetId(null) }}
        sessionName={deleteTargetId ? (sessions?.find(s => s.id === deleteTargetId)?.channelName || sessions?.find(s => s.id === deleteTargetId)?.title || deleteTargetId.slice(0, 10)) : ""}
        onConfirm={confirmDeleteSession}
      />
    </div>
  )
}

function CommandMetric({ icon: Icon, label, value, detail, tone }: {
  icon: typeof Inbox
  label: string
  value: string
  detail: string
  tone: "cyan" | "green" | "amber" | "violet" | "muted"
}) {
  return (
    <div className="rounded-lg border border-white/10 bg-white/[0.06] p-3">
      <div className="flex items-center justify-between gap-3">
        <span className="text-[11px] font-medium text-slate-400">{label}</span>
        <span className={cn(
          "inline-flex h-7 w-7 items-center justify-center rounded-md",
          tone === "cyan" && "bg-cyan-400/15 text-cyan-200",
          tone === "green" && "bg-emerald-400/15 text-emerald-200",
          tone === "amber" && "bg-amber-400/15 text-amber-200",
          tone === "violet" && "bg-violet-400/15 text-violet-200",
          tone === "muted" && "bg-slate-400/15 text-slate-300"
        )}>
          <Icon className="h-3.5 w-3.5" />
        </span>
      </div>
      <div className="mt-2 text-2xl font-semibold tracking-normal text-white">{value}</div>
      <div className="mt-1 truncate text-[11px] text-slate-400">{detail}</div>
    </div>
  )
}
