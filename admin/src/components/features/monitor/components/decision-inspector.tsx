import { useState, useMemo, useCallback } from "react"
import { PanelRightClose, Archive, RefreshCw, ChevronRight, AlertCircle, ChevronsDown, ChevronsUp, Route, ListChecks } from "lucide-react"
import { cn } from "@/lib/utils"
import { useQuery } from "@tanstack/react-query"
import { tracesApi } from "@/api/traces"
import type { ExecutionTrace, MessageLifecycleEvent } from "@/api/types"
import { splitIntoRounds } from "../lib/build-timeline"
import { TraceStatsBar } from "./trace-stats-bar"
import { RoundDetail } from "./round-detail"

export function TraceContent({ trace, isRunning, onViewSubagentTrace, defaultExpanded, expandKey }: {
  trace: ExecutionTrace; isRunning: boolean
  onViewSubagentTrace?: (subTraceId: string, name: string) => void
  defaultExpanded?: boolean; expandKey?: number
}) {
  const [activeRound, setActiveRound] = useState(0)

  const rounds = useMemo(() => splitIntoRounds(trace.steps ?? []), [trace])
  const hasMultipleRounds = rounds.length > 1
  const compactSteps = (trace.steps ?? []).filter(s => s.type === "compact")

  return (
    <>
      <TraceStatsBar trace={trace} />

      {compactSteps.length > 0 && (
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-amber-50 border border-amber-200/60 text-[11px] text-amber-700">
          <Archive className="h-3.5 w-3.5 shrink-0" />
          {compactSteps.map((s, i) => (
            <span key={i}>
              上下文压缩: {s.tokensBefore?.toLocaleString()} → {s.tokensAfter?.toLocaleString()} tokens
              {s.archivedCount ? ` (${s.archivedCount} 条归档)` : ""}
            </span>
          ))}
        </div>
      )}

      {hasMultipleRounds && (
        <div className="flex gap-1 border-b border-slate-200">
          {rounds.map((round, idx) => (
            <button key={idx}
              onClick={() => setActiveRound(idx)}
              className={cn(
                "px-3 py-1.5 text-[11px] font-medium border-b-2 transition-colors",
                activeRound === idx
                  ? "border-brand-600 text-brand-700"
                  : "border-transparent text-slate-500 hover:text-slate-700"
              )}>
              Round {round.roundNumber}
              {round.absorbedCount ? ` (+${round.absorbedCount})` : ""}
            </button>
          ))}
        </div>
      )}

      {rounds.length > 0 && (
        <RoundDetail
          key={`${trace.id}-${activeRound}-${expandKey ?? 0}`}
          steps={rounds[hasMultipleRounds ? activeRound : 0]?.steps ?? []}
          traceId={trace.id}
          isRunning={isRunning}
          onViewSubagentTrace={onViewSubagentTrace}
          defaultExpanded={defaultExpanded}
        />
      )}
    </>
  )
}

export function DecisionInspector({ trace, lifecycleEvents = [], selectedMessageId, isSessionProcessing, onCollapse, onRefreshTrace, isRefreshingTrace }: {
  trace: ExecutionTrace | null
  lifecycleEvents?: MessageLifecycleEvent[]
  selectedMessageId?: number
  isSessionProcessing?: boolean
  onCollapse: () => void
  onRefreshTrace?: () => void
  isRefreshingTrace?: boolean
}) {
  const [subagentStack, setSubagentStack] = useState<Array<{ traceId: string; name: string }>>([])
  const [expandAll, setExpandAll] = useState<boolean | null>(null)
  const [expandKey, setExpandKey] = useState(0)
  const [tab, setTab] = useState<"lifecycle" | "execution">("execution")

  const currentSubagent = subagentStack.length > 0 ? subagentStack[subagentStack.length - 1] : null

  const {
    data: subagentTrace = null,
    isLoading: isLoadingSubagent,
    error: subagentError,
  } = useQuery<ExecutionTrace | null>({
    queryKey: ["traces", currentSubagent?.traceId],
    queryFn: async () => {
      if (!currentSubagent?.traceId) return null
      const res = await tracesApi.getById(currentSubagent.traceId)
      return res.data ?? null
    },
    enabled: !!currentSubagent?.traceId,
    staleTime: 30_000,
  })

  const handleViewSubagentTrace = useCallback((subTraceId: string, name: string) => {
    setSubagentStack(prev => [...prev, { traceId: subTraceId, name }])
  }, [])

  const handleBackToLevel = useCallback((levelIndex: number) => {
    if (levelIndex < 0) {
      setSubagentStack([])
    } else {
      setSubagentStack(prev => prev.slice(0, levelIndex + 1))
    }
  }, [])

  const isRunning = trace?.status === "running" && !!isSessionProcessing
  const visibleLifecycleEvents = lifecycleEvents.filter((event) => {
    if (selectedMessageId) return event.messageId === selectedMessageId || (!!trace?.id && event.traceId === trace.id)
    if (trace?.id) return event.traceId === trace.id
    return true
  })

  if (!trace && visibleLifecycleEvents.length === 0) {
    return (
      <div className="flex flex-col h-full">
        <div className="flex h-12 items-center justify-between border-b border-slate-200 bg-white px-4 shrink-0">
          <div className="flex items-center gap-2">
            <Route className="h-4 w-4 text-slate-500" />
            <h3 className="text-sm font-semibold text-slate-900">处理详情</h3>
          </div>
          <button onClick={onCollapse} className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
            <PanelRightClose className="h-4 w-4" />
          </button>
        </div>
        <div className="flex-1 flex items-center justify-center px-6">
          <div className="max-w-sm rounded-lg border border-dashed border-slate-200 bg-slate-50 px-5 py-6 text-center">
            <ListChecks className="mx-auto mb-3 h-7 w-7 text-slate-300" />
            <p className="text-sm font-medium text-slate-700">还没有选中执行过程</p>
            <p className="mt-1 text-xs leading-relaxed text-slate-400">点击左侧消息后，这里会显示模型、工具、错误和返回路径。</p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex h-12 items-center justify-between border-b border-slate-200 bg-white px-4 shrink-0">
        <div className="flex items-center gap-2">
          <Route className="h-4 w-4 text-slate-500" />
          <h3 className="text-sm font-semibold text-slate-900">执行过程</h3>
        </div>
        <div className="flex items-center gap-1">
          {onRefreshTrace && subagentStack.length === 0 && (
            <button
              onClick={onRefreshTrace}
              disabled={isRefreshingTrace}
              className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600 disabled:opacity-40 transition-colors"
              title="刷新链路"
            >
              <RefreshCw className={cn("h-3.5 w-3.5", isRefreshingTrace && "animate-spin")} />
            </button>
          )}
          <button
            onClick={() => { setExpandAll(true); setExpandKey(k => k + 1) }}
            className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600"
            title="全部展开"
          >
            <ChevronsDown className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={() => { setExpandAll(false); setExpandKey(k => k + 1) }}
            className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600"
            title="全部折叠"
          >
            <ChevronsUp className="h-3.5 w-3.5" />
          </button>
          <button onClick={onCollapse} className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
            <PanelRightClose className="h-4 w-4" />
          </button>
        </div>
      </div>

      {subagentStack.length > 0 && (
        <div className="flex items-center gap-1 px-4 py-2 border-b border-slate-100 bg-slate-50 text-[12px] shrink-0 overflow-x-auto">
          <button onClick={() => handleBackToLevel(-1)} className="text-brand-600 hover:text-brand-800 font-medium shrink-0">
            主 Trace
          </button>
          {subagentStack.map((entry, idx) => {
            const isLast = idx === subagentStack.length - 1
            return (
              <span key={entry.traceId} className="flex items-center gap-1 min-w-0">
                <ChevronRight className="h-3 w-3 text-slate-400 shrink-0" />
                {isLast ? (
                  <span className="text-slate-700 font-medium truncate">Subagent: {entry.name}</span>
                ) : (
                  <button onClick={() => handleBackToLevel(idx)} className="text-brand-600 hover:text-brand-800 font-medium truncate">
                    Subagent: {entry.name}
                  </button>
                )}
              </span>
            )
          })}
        </div>
      )}

      {subagentStack.length === 0 && (
        <div className="border-b border-slate-200 bg-white px-4 py-3 shrink-0">
          <InspectorSummary trace={trace} events={visibleLifecycleEvents} />
          <div className="mt-3 flex gap-1">
          <button
            onClick={() => setTab("execution")}
            className={cn(
              "rounded-md px-3 py-1.5 text-[11px] font-medium transition-colors",
              tab === "execution" ? "bg-slate-900 text-white" : "bg-slate-100 text-slate-500 hover:text-slate-700"
            )}
          >
            执行流
          </button>
          <button
            onClick={() => setTab("lifecycle")}
            className={cn(
              "rounded-md px-3 py-1.5 text-[11px] font-medium transition-colors",
              tab === "lifecycle" ? "bg-slate-900 text-white" : "bg-slate-100 text-slate-500 hover:text-slate-700"
            )}
          >
            接收记录
          </button>
          </div>
        </div>
      )}

      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {!currentSubagent && tab === "lifecycle" ? (
          <LifecycleEventList events={visibleLifecycleEvents} hasTrace={!!trace} onSwitchToExecution={() => setTab("execution")} />
        ) : currentSubagent ? (
          isLoadingSubagent ? (
            <div className="flex items-center justify-center h-32 text-sm text-slate-400">
              加载 Subagent 执行记录...
            </div>
          ) : subagentError || !subagentTrace ? (
            <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-red-50 border border-red-200/60 text-[12px] text-red-600">
              <AlertCircle className="h-4 w-4 shrink-0" />
              无法加载 subagent 执行记录
            </div>
          ) : (
            <TraceContent trace={subagentTrace} isRunning={false} onViewSubagentTrace={handleViewSubagentTrace} defaultExpanded={expandAll ?? undefined} expandKey={expandKey} />
          )
        ) : trace ? (
          <TraceContent trace={trace} isRunning={isRunning} onViewSubagentTrace={handleViewSubagentTrace} defaultExpanded={expandAll ?? undefined} expandKey={expandKey} />
        ) : (
          <div className="text-sm text-slate-400 text-center py-12">暂无执行流记录</div>
        )}
      </div>
    </div>
  )
}

const STAGE_LABELS: Record<string, string> = {
  dispatch_accepted: "派发接收",
  message_saved: "消息入库",
  session_event_appended: "进入队列",
  worker_notified: "通知 worker",
  worker_started: "worker 启动",
  event_drained: "事件消费",
  skill_context_built: "Skill 上下文",
  context_built: "进入上下文",
  outbound_send_requested: "请求发送",
  outbound_send_completed: "发送完成",
  processed_completed: "处理完成",
}

function InspectorSummary({ trace, events }: { trace: ExecutionTrace | null; events: MessageLifecycleEvent[] }) {
  const failed = events.some((event) => event.status === "failed")
  const completed = [...events].reverse().find((event) => event.stage === "processed_completed")
  const outcome = completed?.outcome
  const traceStatus = trace?.status

  return (
    <div className="grid grid-cols-3 gap-2">
      <SummaryCell label="执行流" value={trace ? traceStatus || "可查看" : "无记录"} tone={trace ? "green" : "muted"} />
      <SummaryCell
        label="结果"
        value={failed ? "失败" : outcome === "replied" ? "已回复" : outcome === "no_reply" ? "未回复" : outcome === "send_failed" ? "发送失败" : trace?.status === "running" ? "处理中" : "待确认"}
        tone={failed || outcome === "send_failed" ? "red" : outcome === "no_reply" ? "amber" : outcome || trace ? "green" : "muted"}
      />
      <SummaryCell label="接收记录" value={events.length > 0 ? `${events.length} 条` : "无记录"} tone={events.length > 0 ? "green" : "muted"} />
    </div>
  )
}

function SummaryCell({ label, value, tone }: { label: string; value: string; tone: "muted" | "green" | "amber" | "red" }) {
  return (
    <div className={cn(
      "rounded-md border px-2 py-2",
      tone === "green" && "border-emerald-200 bg-emerald-50",
      tone === "amber" && "border-amber-200 bg-amber-50",
      tone === "red" && "border-red-200 bg-red-50",
      tone === "muted" && "border-slate-200 bg-slate-50"
    )}>
      <div className="text-[10px] text-slate-400">{label}</div>
      <div className={cn(
        "mt-0.5 truncate text-[12px] font-semibold",
        tone === "green" && "text-emerald-700",
        tone === "amber" && "text-amber-700",
        tone === "red" && "text-red-700",
        tone === "muted" && "text-slate-500"
      )}>{value}</div>
    </div>
  )
}

function LifecycleEventList({ events, hasTrace, onSwitchToExecution }: { events: MessageLifecycleEvent[]; hasTrace: boolean; onSwitchToExecution: () => void }) {
  if (events.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-4 py-6 text-center">
        <AlertCircle className="mx-auto mb-2 h-6 w-6 text-slate-300" />
        <p className="text-sm font-medium text-slate-700">暂无接收记录</p>
        <p className="mt-1 text-xs leading-relaxed text-slate-400">
          这通常代表旧数据还没有生命周期采集，不等于没有收到消息。
        </p>
        {hasTrace && (
          <button
            onClick={onSwitchToExecution}
            className="mt-3 rounded-md bg-slate-900 px-3 py-1.5 text-[12px] font-medium text-white hover:bg-slate-800"
          >
            查看执行流
          </button>
        )}
      </div>
    )
  }
  return (
    <div className="space-y-2">
      {events.map((event) => (
        <div
          key={event.id}
          className={cn(
            "rounded-md border px-3 py-2 text-[12px]",
            event.status === "failed" ? "border-red-200 bg-red-50" : "border-slate-200 bg-white"
          )}
        >
          <div className="flex items-center justify-between gap-2">
            <span className={cn("font-medium", event.status === "failed" ? "text-red-700" : "text-slate-800")}>
              {STAGE_LABELS[event.stage] ?? event.stage}
            </span>
            {event.outcome && (
              <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-600">
                {event.outcome === "replied" ? "已回复" : event.outcome === "no_reply" ? "未回复" : event.outcome}
              </span>
            )}
          </div>
          {event.summary && <p className="mt-1 text-slate-500">{event.summary}</p>}
          {event.payload && Object.keys(event.payload).length > 0 && (
            <pre className="mt-2 max-h-36 overflow-auto rounded bg-slate-50 p-2 text-[11px] text-slate-600">
              {JSON.stringify(event.payload, null, 2)}
            </pre>
          )}
        </div>
      ))}
    </div>
  )
}
