import { useState, useMemo } from "react"
import {
  Brain, Wrench, CheckCircle2, XCircle, Clock, ChevronDown, ChevronRight,
  Zap, FileText, ExternalLink, AlertTriangle, RotateCw, Shield,
} from "lucide-react"
import { cn, formatDuration, formatCost, truncate } from "@/lib/utils"
import type { ExecutionStep } from "@/api/types"
import type { FlatEvent } from "../lib/types"
import { flattenSteps } from "../lib/build-timeline"
import { LLMIOViewer } from "./llm-io-viewer"
import { SubagentStepCard } from "./subagent-step-card"
import { safePretty, openJsonInNewTab } from "../lib/json-utils"

// --- Model Output Row ---
function ModelOutputRow({ event, traceId, defaultExpanded }: {
  event: Extract<FlatEvent, { type: "model-output" }>; traceId?: string
  defaultExpanded?: boolean
}) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? false)
  const [llmIOOpen, setLlmIOOpen] = useState(false)
  const { thinkings, llmCall } = event

  const thinkingText = thinkings.map(t => t.thinking ?? "").join("\n").trim()
  const modelShort = llmCall?.model?.replace(/^claude-/, "").replace(/-\d{8}$/, "") ?? null
  const hasLLMIO = !!llmCall?.llmIORef && !!traceId

  return (
    <div className="group/row">
      <div
        className="flex items-start gap-2.5 py-1.5 px-2 rounded-md cursor-pointer hover:bg-slate-50 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex h-5 w-5 items-center justify-center rounded-full bg-slate-100 shrink-0 mt-0.5">
          <Brain className="h-3 w-3 text-slate-500" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">模型输出</span>
            {modelShort && <span className="text-[10px] text-slate-400 font-mono">{modelShort}</span>}
            {llmCall && (
              <span className="flex items-center gap-0.5 text-[10px] text-slate-400">
                <Zap className="h-2.5 w-2.5" />{llmCall.inputTokens ?? "?"}↑{llmCall.outputTokens ?? "?"}↓
              </span>
            )}
            {llmCall?.durationMs != null && (
              <span className="flex items-center gap-0.5 text-[10px] text-slate-400">
                <Clock className="h-2.5 w-2.5" />{formatDuration(llmCall.durationMs)}
              </span>
            )}
            {llmCall?.costUsd != null && Number(llmCall.costUsd) > 0 && (
              <span className="text-[10px] text-slate-400 ml-auto">{formatCost(llmCall.costUsd)}</span>
            )}
            <span className="text-slate-300 ml-auto">
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </span>
          </div>
          {!expanded && thinkingText && (
            <p className="text-[12px] text-slate-500 mt-0.5 truncate">{truncate(thinkingText, 80)}</p>
          )}
        </div>
      </div>

      {expanded && (
        <div className="ml-7 pl-2.5 border-l-2 border-slate-200 mt-0.5 mb-2 space-y-2">
          {thinkingText && (
            <p className="text-[13px] text-slate-600 leading-relaxed wrap-break-word whitespace-pre-wrap">
              {thinkingText}
            </p>
          )}
          {hasLLMIO && (
            <button
              onClick={(e) => { e.stopPropagation(); setLlmIOOpen(true) }}
              className="flex items-center gap-1 px-2 py-0.5 rounded text-[10px] text-brand-600 hover:bg-brand-50 transition-colors"
            >
              <FileText className="h-3 w-3" />查看完整 LLM I/O
            </button>
          )}
        </div>
      )}

      {llmIOOpen && hasLLMIO && (
        <LLMIOViewer traceId={traceId!} llmIORef={llmCall!.llmIORef!} onClose={() => setLlmIOOpen(false)} />
      )}
    </div>
  )
}

// --- Tool Call Row ---
function ToolCallRow({ step, defaultExpanded }: { step: ExecutionStep; defaultExpanded?: boolean }) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? false)
  const inputPretty = useMemo(() => safePretty(step.toolInput), [step.toolInput])

  return (
    <div className="group/row">
      <div
        className="flex items-start gap-2.5 py-1.5 px-2 rounded-md cursor-pointer hover:bg-slate-50 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex h-5 w-5 items-center justify-center rounded-full bg-brand-50 shrink-0 mt-0.5">
          <Wrench className="h-3 w-3 text-brand-600" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-brand-600">工具执行</span>
            <span className="text-[13px] font-medium text-slate-700">{step.toolName}</span>
            <span className="text-slate-300 ml-auto">
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </span>
          </div>
        </div>
      </div>

      {expanded && step.toolInput != null && (
        <div className="ml-7 pl-2.5 border-l-2 border-brand-200 mt-0.5 mb-2">
          <div className="rounded-md border border-brand-100 bg-brand-50/40 overflow-hidden">
            <div className="px-2.5 py-0.5 bg-brand-100/50 text-[10px] font-semibold text-brand-700 uppercase tracking-wider flex items-center">
              <span>输入</span>
              <button className="ml-auto inline-flex items-center gap-1 text-[10px] font-medium text-brand-700 hover:text-brand-900"
                onClick={(e) => { e.stopPropagation(); openJsonInNewTab(`Tool Input · ${step.toolName}`, step.toolInput) }}>
                <ExternalLink className="h-3 w-3" />全文
              </button>
            </div>
            <div className="p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-40 overflow-y-auto">
              {inputPretty}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// --- Tool Result Row ---
function ToolResultRow({ step, callStep, defaultExpanded }: { step: ExecutionStep; callStep?: ExecutionStep; defaultExpanded?: boolean }) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? step.toolSuccess === false)
  const success = step.toolSuccess !== false
  const resultPretty = useMemo(() => safePretty(step.error || step.toolResult), [step])
  const toolName = step.toolName || callStep?.toolName || "unknown"

  return (
    <div className="group/row">
      <div
        className="flex items-start gap-2.5 py-1.5 px-2 rounded-md cursor-pointer hover:bg-slate-50 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className={cn(
          "flex h-5 w-5 items-center justify-center rounded-full shrink-0 mt-0.5",
          success ? "bg-green-50" : "bg-red-50"
        )}>
          {success
            ? <CheckCircle2 className="h-3 w-3 text-green-600" />
            : <XCircle className="h-3 w-3 text-red-600" />
          }
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <span className={cn(
              "text-[10px] font-semibold uppercase tracking-wider",
              success ? "text-green-600" : "text-red-600"
            )}>工具结果</span>
            <span className="text-[12px] text-slate-500">{toolName}</span>
            {step.toolDuration != null && (
              <span className="flex items-center gap-0.5 text-[10px] text-slate-400">
                <Clock className="h-2.5 w-2.5" />{formatDuration(step.toolDuration)}
              </span>
            )}
            <span className="text-slate-300 ml-auto">
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </span>
          </div>
          {!expanded && resultPretty && (
            <p className="text-[12px] text-slate-500 mt-0.5 truncate">{truncate(resultPretty, 80)}</p>
          )}
        </div>
      </div>

      {expanded && (resultPretty || step.error) && (
        <div className="ml-7 pl-2.5 border-l-2 border-slate-200 mt-0.5 mb-2">
          <div className={cn("rounded-md border overflow-hidden",
            success ? "border-green-100 bg-green-50/40" : "border-red-100 bg-red-50/40")}>
            <div className={cn("px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider flex items-center",
              success ? "bg-green-100/50 text-green-700" : "bg-red-100/50 text-red-700")}>
              <span>{success ? "结果" : "错误"}</span>
              {step.toolResult != null && !step.error && (
                <button className={cn("ml-auto inline-flex items-center gap-1 text-[10px] font-medium",
                  success ? "text-green-700 hover:text-green-900" : "text-red-700 hover:text-red-900")}
                  onClick={(e) => { e.stopPropagation(); openJsonInNewTab(`Tool Result · ${toolName}`, step.toolResult) }}>
                  <ExternalLink className="h-3 w-3" />全文
                </button>
              )}
            </div>
            <div className="p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-40 overflow-y-auto">
              {step.error ? <span className="text-red-600">{step.error}</span> : resultPretty}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// --- Error Row ---
function ErrorRow({ step, defaultExpanded }: { step: ExecutionStep; defaultExpanded?: boolean }) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? true)

  return (
    <div className="group/row">
      <div
        className="flex items-start gap-2.5 py-1.5 px-2 rounded-md cursor-pointer hover:bg-red-50/50 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex h-5 w-5 items-center justify-center rounded-full bg-red-50 shrink-0 mt-0.5">
          <AlertTriangle className="h-3 w-3 text-red-600" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-red-600">错误</span>
            <span className="text-slate-300 ml-auto">
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </span>
          </div>
          {!expanded && step.error && (
            <p className="text-[12px] text-red-500 mt-0.5 truncate">{truncate(step.error, 80)}</p>
          )}
        </div>
      </div>

      {expanded && step.error && (
        <div className="ml-7 pl-2.5 border-l-2 border-red-200 mt-0.5 mb-2">
          <div className="rounded-md border border-red-100 bg-red-50/40 p-2.5 text-[11px] font-mono text-red-600 max-h-48 overflow-y-auto whitespace-pre-wrap">
            {step.error}
          </div>
        </div>
      )}
    </div>
  )
}

// --- Mode Change Row ---
function ModeChangeRow({ step }: { step: ExecutionStep }) {
  const isPlanEnter = step.modeTo === "plan"
  return (
    <div className={cn(
      "flex items-center gap-2 px-3 py-1.5 rounded-md border text-[11px]",
      isPlanEnter
        ? "bg-amber-50 border-amber-200/60 text-amber-700"
        : "bg-emerald-50 border-emerald-200/60 text-emerald-700"
    )}>
      <Shield className="h-3.5 w-3.5 shrink-0" />
      <span>{step.content || (isPlanEnter ? "进入计划模式" : "退出计划模式")}</span>
      {step.modeFrom && step.modeTo && (
        <span className="text-[10px] opacity-70 ml-auto">{step.modeFrom} → {step.modeTo}</span>
      )}
    </div>
  )
}

// --- Subagent Reentry Row ---
function SubagentReentryRow({ step }: { step: ExecutionStep }) {
  return (
    <div className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-indigo-50 border border-indigo-200/60 text-[11px] text-indigo-700">
      <RotateCw className="h-3.5 w-3.5 shrink-0" />
      <span>{step.content || "Subagent Re-entry"}</span>
    </div>
  )
}

// --- Flat Event Row (dispatcher) ---
function FlatEventRow({ event, traceId, allEvents, onViewSubagentTrace, defaultExpanded }: {
  event: FlatEvent; traceId?: string; allEvents?: FlatEvent[];
  onViewSubagentTrace?: (subTraceId: string, name: string) => void
  defaultExpanded?: boolean
}) {
  switch (event.type) {
    case "model-output":
      return <ModelOutputRow event={event} traceId={traceId} defaultExpanded={defaultExpanded} />
    case "tool-call":
      if (event.step.toolName === "run_subagent_async" && allEvents) {
        const resultEvent = allEvents.find(
          (e): e is Extract<FlatEvent, { type: "tool-result" }> =>
            e.type === "tool-result" && e.step.toolCallId === event.step.toolCallId
        )
        return (
          <SubagentStepCard
            callStep={event.step}
            resultStep={resultEvent?.step}
            onViewTrace={onViewSubagentTrace}
          />
        )
      }
      return <ToolCallRow step={event.step} defaultExpanded={defaultExpanded} />
    case "tool-result":
      if (event.step.toolName === "run_subagent_async") return null
      return <ToolResultRow step={event.step} callStep={event.callStep} defaultExpanded={defaultExpanded} />
    case "error":
      return <ErrorRow step={event.step} defaultExpanded={defaultExpanded} />
    case "subagent-reentry":
      return <SubagentReentryRow step={event.step} />
    case "mode-change":
      return <ModeChangeRow step={event.step} />
  }
}

// --- Main Component ---
export function RoundDetail({ steps, traceId, isRunning, onViewSubagentTrace, defaultExpanded }: {
  steps: ExecutionStep[]; traceId?: string; isRunning?: boolean;
  onViewSubagentTrace?: (subTraceId: string, name: string) => void
  defaultExpanded?: boolean
}) {
  const flatEvents = useMemo(() => flattenSteps(steps), [steps])

  if (flatEvents.length === 0) {
    return <div className="text-xs text-slate-400 py-2 px-2">暂无事件</div>
  }

  return (
    <div className="space-y-0.5">
      {flatEvents.map((event, i) => (
        <FlatEventRow key={i} event={event} traceId={traceId} allEvents={flatEvents} onViewSubagentTrace={onViewSubagentTrace} defaultExpanded={defaultExpanded} />
      ))}
      {isRunning && (
        <div className="flex items-center gap-2 py-2 px-2 text-xs text-slate-400">
          <span className="h-1.5 w-1.5 rounded-full bg-brand-500 animate-live-pulse" />
          处理中...
        </div>
      )}
    </div>
  )
}
