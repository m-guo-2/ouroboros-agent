import { Clock, Zap, DollarSign, Cpu, RotateCw } from "lucide-react"
import { formatDuration, formatCost } from "@/lib/utils"
import type { ExecutionTrace } from "@/api/types"

export function TraceStatsBar({ trace }: { trace: ExecutionTrace }) {
  const duration = trace.completedAt
    ? trace.completedAt - trace.startedAt
    : Date.now() - trace.startedAt
  const isRunning = trace.status === "running"

  const llmCalls = trace.steps.filter(s => s.type === "llm_call").length
  const iterations = new Set(trace.steps.filter(s => s.iteration > 0).map(s => s.iteration)).size
  const absorbCount = trace.steps.filter(s => s.type === "absorb").length

  return (
    <div className="flex flex-wrap items-center gap-3 px-3 py-2 rounded-lg bg-slate-50 border border-slate-200 text-[11px] text-slate-500">
      <span className="flex items-center gap-1">
        <Clock className="h-3 w-3" />
        {isRunning && <span className="h-1.5 w-1.5 rounded-full bg-green-500 animate-pulse" />}
        {formatDuration(duration)}
      </span>
      <span className="flex items-center gap-1">
        <Zap className="h-3 w-3" />
        {trace.inputTokens.toLocaleString()}↑ {trace.outputTokens.toLocaleString()}↓
      </span>
      {trace.totalCostUsd > 0 && (
        <span className="flex items-center gap-1">
          <DollarSign className="h-3 w-3" />{formatCost(trace.totalCostUsd)}
        </span>
      )}
      <span className="flex items-center gap-1">
        <Cpu className="h-3 w-3" />{llmCalls} 调用
      </span>
      {iterations > 0 && <span>{iterations} 迭代</span>}
      {absorbCount > 0 && (
        <span className="flex items-center gap-1 text-amber-600">
          <RotateCw className="h-3 w-3" />{absorbCount + 1} 轮处理
        </span>
      )}
    </div>
  )
}
