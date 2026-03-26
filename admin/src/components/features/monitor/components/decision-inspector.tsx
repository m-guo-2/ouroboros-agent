import { useState, useMemo, useCallback } from "react"
import { PanelRightClose, Archive, RefreshCw, ChevronRight, AlertCircle, ChevronsDown, ChevronsUp } from "lucide-react"
import { cn } from "@/lib/utils"
import { useQuery } from "@tanstack/react-query"
import { tracesApi } from "@/api/traces"
import type { ExecutionTrace } from "@/api/types"
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

export function DecisionInspector({ trace, isSessionProcessing, onCollapse, onRefreshTrace, isRefreshingTrace }: {
  trace: ExecutionTrace | null
  isSessionProcessing?: boolean
  onCollapse: () => void
  onRefreshTrace?: () => void
  isRefreshingTrace?: boolean
}) {
  const [subagentStack, setSubagentStack] = useState<Array<{ traceId: string; name: string }>>([])
  const [expandAll, setExpandAll] = useState<boolean | null>(null)
  const [expandKey, setExpandKey] = useState(0)

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

  if (!trace) {
    return (
      <div className="flex flex-col h-full">
        <div className="flex items-center justify-between px-4 py-3 border-b border-slate-200 bg-white shrink-0">
          <h3 className="text-sm font-semibold text-slate-900">决策详情</h3>
          <button onClick={onCollapse} className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
            <PanelRightClose className="h-4 w-4" />
          </button>
        </div>
        <div className="flex-1 flex items-center justify-center text-center px-6">
          <p className="text-sm text-slate-400">点击对话中的 Agent 回复，查看完整决策过程</p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-3 border-b border-slate-200 bg-white shrink-0">
        <h3 className="text-sm font-semibold text-slate-900">决策详情</h3>
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

      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {currentSubagent ? (
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
        ) : (
          <TraceContent trace={trace} isRunning={isRunning} onViewSubagentTrace={handleViewSubagentTrace} defaultExpanded={expandAll ?? undefined} expandKey={expandKey} />
        )}
      </div>
    </div>
  )
}
