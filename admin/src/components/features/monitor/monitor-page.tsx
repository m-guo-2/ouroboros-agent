import { useState, useMemo, useCallback } from "react"
import { Activity, PanelRight } from "lucide-react"
import { useMonitorSessions } from "@/hooks/use-monitor"
import { useSession, useSessionMessages, useDeleteSession } from "@/hooks/use-sessions"
import { useQueries } from "@tanstack/react-query"
import { tracesApi } from "@/api/traces"
import type { ExecutionTrace } from "@/api/types"
import { useSessionCompactions } from "./hooks/use-session-compactions"
import { buildExchanges } from "./lib/build-timeline"
import { SessionList } from "./components/session-list"
import { ConversationTimeline } from "./components/conversation-timeline"
import { DecisionInspector } from "./components/decision-inspector"

export function MonitorPage() {
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null)
  const [selectedExchangeIndex, setSelectedExchangeIndex] = useState<number | null>(null)
  const [search, setSearch] = useState("")
  const [inspectorOpen, setInspectorOpen] = useState(true)
  const { data: sessions, isLoading, refetch: refetchSessions, isFetching: isRefreshingSessions } = useMonitorSessions()
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
  const { data: session } = useSession(effectiveSessionId ?? "")
  const isProcessing = session?.executionStatus === "processing"
  const { data: messages = [], isLoading: isLoadingMessages } = useSessionMessages(effectiveSessionId ?? "", {
    refetchInterval: isProcessing ? 1000 : false,
  })

  // Compactions
  const { data: compactions = [] } = useSessionCompactions(effectiveSessionId)

  // Traces
  const traceIds = useMemo(() => {
    return [...new Set(messages.map((m) => m.traceId).filter(Boolean) as string[])]
  }, [messages])

  const activeTraceId = isProcessing ? traceIds[traceIds.length - 1] : undefined
  const traceQueryResults = useQueries({
    queries: traceIds.map((tid) => ({
      queryKey: ["traces", tid],
      queryFn: () => tracesApi.getById(tid).then((r) => r.data ?? null),
      staleTime: Infinity,
      refetchInterval: tid === activeTraceId ? 2000 : false,
    })),
  })

  const fullTraces = useMemo(() => {
    return traceQueryResults.reduce<Record<string, ExecutionTrace>>((acc, q) => {
      if (q.data) acc[q.data.id] = q.data
      return acc
    }, {})
  }, [traceQueryResults])

  // Build exchanges
  const exchanges = useMemo(() => {
    if (messages.length === 0) return []
    return buildExchanges(messages, fullTraces)
  }, [messages, fullTraces])

  const effectiveExchangeIndex = useMemo(() => {
    if (selectedExchangeIndex != null) return selectedExchangeIndex
    if (isProcessing && exchanges.length > 0) return exchanges[exchanges.length - 1].exchangeIndex
    return null
  }, [selectedExchangeIndex, isProcessing, exchanges])

  const selectedTrace = useMemo(() => {
    if (effectiveExchangeIndex == null) return null
    const exchange = exchanges.find(e => e.exchangeIndex === effectiveExchangeIndex)
    return exchange?.trace ?? null
  }, [effectiveExchangeIndex, exchanges])

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
        onRefresh={() => refetchSessions()}
        isRefreshing={isRefreshingSessions}
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
                  {messages.length} 条消息 · {exchanges.length} 次交互
                  {compactions.length > 0 && ` · ${compactions.length} 次压缩`}
                </p>
              </div>
              {!inspectorOpen && (
                <button onClick={() => setInspectorOpen(true)}
                  className="p-1.5 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
                  <PanelRight className="h-4 w-4" />
                </button>
              )}
            </div>

            <ConversationTimeline
              exchanges={exchanges}
              compactions={compactions}
              traces={fullTraces}
              isProcessing={!!isProcessing}
              selectedExchangeIndex={effectiveExchangeIndex}
              onSelectExchange={handleSelectExchange}
              isLoadingMessages={isLoadingMessages}
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
            trace={selectedTrace}
            isSessionProcessing={isProcessing}
            onCollapse={() => setInspectorOpen(false)}
          />
        </div>
      )}
    </div>
  )
}
