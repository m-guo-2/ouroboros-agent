import { useState } from "react"
import { ChevronDown, ChevronRight, Zap, Clock, FileText, CheckCircle2, XCircle } from "lucide-react"
import { cn, formatDuration, formatCost, truncate } from "@/lib/utils"
import type { ExecutionStep } from "@/api/types"
import type { IterationData } from "../lib/types"
import { groupStepsByIteration } from "../lib/build-timeline"
import { ThinkingView } from "./thinking-view"
import { ToolCard } from "./tool-card"
import { ModelOutputView } from "./model-output-view"
import { LLMIOViewer } from "./llm-io-viewer"

function IterationGroup({ data, traceId, defaultExpanded }: {
  data: IterationData; traceId?: string; defaultExpanded: boolean
}) {
  const [expanded, setExpanded] = useState(defaultExpanded)
  const [llmIOOpen, setLlmIOOpen] = useState(false)
  const { llmCall, systemSteps, thinkings, toolPairs, contentSteps, errorSteps } = data
  const hasErrors = errorSteps.length > 0 || toolPairs.some(p => p.result?.toolSuccess === false)
  const isSystemOnly = data.iteration === 0
  const hasLLMIO = !!llmCall?.llmIORef && !!traceId

  const iterLabel = isSystemOnly ? "初始化" : `Iteration ${data.iteration}`
  const modelShort = llmCall?.model?.replace(/^claude-/, "").replace(/-\d{8}$/, "") ?? null

  return (
    <div className="mb-1.5">
      <div className="flex items-center">
        <button onClick={() => setExpanded(!expanded)}
          className={cn(
            "flex items-center gap-1.5 flex-1 py-1 px-2 rounded-md text-left transition-colors",
            expanded ? "bg-slate-100/80" : "hover:bg-slate-100/60",
            hasErrors && "text-red-700"
          )}>
          {expanded ? <ChevronDown className="h-3 w-3 text-slate-400 flex-shrink-0" /> : <ChevronRight className="h-3 w-3 text-slate-400 flex-shrink-0" />}
          <span className={cn("text-[11px] font-semibold", isSystemOnly ? "text-purple-600" : "text-slate-600")}>
            {iterLabel}
          </span>
          {llmCall && (
            <>
              {modelShort && <span className="text-[10px] text-slate-400 font-mono ml-1">{modelShort}</span>}
              <span className="flex items-center gap-0.5 text-[10px] text-slate-400">
                <Zap className="h-2.5 w-2.5" />{llmCall.inputTokens ?? "?"}↑{llmCall.outputTokens ?? "?"}↓
              </span>
              {llmCall.durationMs != null && (
                <span className="flex items-center gap-0.5 text-[10px] text-slate-400">
                  <Clock className="h-2.5 w-2.5" />{formatDuration(llmCall.durationMs)}
                </span>
              )}
            </>
          )}
          {toolPairs.length > 0 && <span className="text-[10px] text-slate-400 ml-1">{toolPairs.length} 工具</span>}
          {hasErrors && <span className="text-[10px] text-red-500 ml-1">⚠</span>}
          {llmCall?.costUsd != null && llmCall.costUsd > 0 && (
            <span className="ml-auto text-[10px] text-slate-400">{formatCost(llmCall.costUsd)}</span>
          )}
        </button>
        {hasLLMIO && (
          <button onClick={() => setLlmIOOpen(true)}
            className="flex items-center gap-0.5 px-1.5 py-0.5 ml-1 rounded text-[10px] text-slate-400 hover:text-brand-600 hover:bg-brand-50 transition-colors"
            title="查看完整 LLM 请求/响应">
            <FileText className="h-3 w-3" /><span>I/O</span>
          </button>
        )}
      </div>

      {expanded && (
        <div className="ml-3 pl-2.5 border-l-2 border-slate-200 mt-0.5">
          {hasLLMIO && <ModelOutputView traceId={traceId!} llmIORef={llmCall!.llmIORef!} />}
          {systemSteps.map((s, i) => <ThinkingView key={`sys-${i}`} step={s} />)}
          {thinkings.map((s, i) => <ThinkingView key={`think-${i}`} step={s} />)}
          {toolPairs.map((pair, i) => <ToolCard key={`tool-${i}`} pair={pair} />)}
          {contentSteps.map((s, i) => (
            <div key={`content-${i}`} className="flex gap-2.5">
              <div className="flex flex-col items-center pt-0.5">
                <div className="flex h-5 w-5 items-center justify-center rounded-full bg-green-50 flex-shrink-0">
                  <CheckCircle2 className="h-3 w-3 text-green-600" />
                </div>
              </div>
              <div className="flex-1 pb-2 min-w-0">
                <span className="text-[10px] font-semibold uppercase tracking-wider text-green-600">Result</span>
                <p className="text-[13px] text-slate-600 mt-0.5 break-words">{truncate(s.content ?? "", 200)}</p>
              </div>
            </div>
          ))}
          {errorSteps.map((s, i) => (
            <div key={`err-${i}`} className="flex gap-2.5">
              <div className="flex flex-col items-center pt-0.5">
                <div className="flex h-5 w-5 items-center justify-center rounded-full bg-red-50 flex-shrink-0">
                  <XCircle className="h-3 w-3 text-red-600" />
                </div>
              </div>
              <div className="flex-1 pb-2 min-w-0">
                <span className="text-[10px] font-semibold uppercase tracking-wider text-red-600">Error</span>
                <div className="mt-1 rounded-md border border-slate-200 bg-slate-50/80 p-2.5 text-[11px] font-mono text-red-600 max-h-48 overflow-y-auto">
                  {s.error}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {llmIOOpen && hasLLMIO && (
        <LLMIOViewer traceId={traceId!} llmIORef={llmCall!.llmIORef!} onClose={() => setLlmIOOpen(false)} />
      )}
    </div>
  )
}

export function RoundDetail({ steps, traceId, isRunning }: {
  steps: ExecutionStep[]; traceId?: string; isRunning?: boolean
}) {
  const iterGroups = groupStepsByIteration(steps)
  const maxIteration = iterGroups.filter(g => g.iteration > 0).length

  return (
    <div>
      {iterGroups.map((group) => (
        <IterationGroup
          key={group.iteration}
          data={group}
          traceId={traceId}
          defaultExpanded={
            isRunning
              ? group.iteration === iterGroups[iterGroups.length - 1]?.iteration
              : group.iteration > 0 && maxIteration <= 3
          }
        />
      ))}
    </div>
  )
}
