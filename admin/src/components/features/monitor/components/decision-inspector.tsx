import { useState, useMemo } from "react"
import { PanelRightClose, Archive, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"
import type { ExecutionTrace } from "@/api/types"
import { splitIntoRounds } from "../lib/build-timeline"
import { TraceStatsBar } from "./trace-stats-bar"
import { RoundDetail } from "./round-detail"

export function DecisionInspector({ trace, isSessionProcessing, onCollapse, onRefreshTrace, isRefreshingTrace }: {
  trace: ExecutionTrace | null
  isSessionProcessing?: boolean
  onCollapse: () => void
  onRefreshTrace?: () => void
  isRefreshingTrace?: boolean
}) {
  const [activeRound, setActiveRound] = useState(0)

  const rounds = useMemo(() => {
    if (!trace) return []
    return splitIntoRounds(trace.steps)
  }, [trace])

  const isRunning = trace?.status === "running" && !!isSessionProcessing
  const hasMultipleRounds = rounds.length > 1

  if (!trace) {
    return (
      <div className="flex flex-col h-full">
        <div className="flex items-center justify-between px-4 py-3 border-b border-slate-200 bg-white shrink-0">
          <h3 className="text-sm font-semibold text-slate-900">Decision Inspector</h3>
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

  const compactSteps = trace.steps.filter(s => s.type === "compact")

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-3 border-b border-slate-200 bg-white shrink-0">
        <h3 className="text-sm font-semibold text-slate-900">Decision Inspector</h3>
        <div className="flex items-center gap-1">
          {onRefreshTrace && (
            <button
              onClick={onRefreshTrace}
              disabled={isRefreshingTrace}
              className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600 disabled:opacity-40 transition-colors"
              title="刷新链路"
            >
              <RefreshCw className={cn("h-3.5 w-3.5", isRefreshingTrace && "animate-spin")} />
            </button>
          )}
          <button onClick={onCollapse} className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
            <PanelRightClose className="h-4 w-4" />
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-3">
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
            steps={rounds[hasMultipleRounds ? activeRound : 0]?.steps ?? []}
            traceId={trace.id}
            isRunning={isRunning}
          />
        )}
      </div>
    </div>
  )
}
