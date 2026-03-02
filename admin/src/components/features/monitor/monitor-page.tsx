import { useState, useEffect, useRef, useMemo, useCallback } from "react"
import {
  Activity, MessageSquare,
  User, Bot, Brain, Wrench, CheckCircle2, XCircle,
  Clock, ChevronDown, ChevronRight, Search, Trash2,
  Cpu, Settings2, Zap, FileText, X
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/shared/status-badge"
import { ChannelBadge } from "@/components/shared/channel-badge"
import { MarkdownContent } from "@/components/shared/markdown-content"
import { useMonitorSessions } from "@/hooks/use-monitor"
import { useSession, useSessionMessages, useDeleteSession } from "@/hooks/use-sessions"
import { useQuery } from "@tanstack/react-query"
import { tracesApi } from "@/api/traces"
import { cn, timeAgo, formatDuration, formatCost, truncate } from "@/lib/utils"
import type { MessageData, ExecutionTrace, ExecutionStep } from "@/api/types"

// ===== Message Exchange: user msg → trace → assistant response =====

interface MessageExchange {
  userMessage: Pick<MessageData, "role" | "content"> & Partial<MessageData>
  trace?: ExecutionTrace
  assistantMessage?: MessageData
  isSystemInitiated?: boolean
}

function buildExchanges(
  messages: MessageData[],
  traces: Record<string, ExecutionTrace>,
): MessageExchange[] {
  const exchanges: MessageExchange[] = []
  let i = 0

  while (i < messages.length) {
    const msg = messages[i]

    if (msg.role === "user") {
      const exchange: MessageExchange = { userMessage: msg }

      if (i + 1 < messages.length && messages[i + 1].role === "assistant") {
        exchange.assistantMessage = messages[i + 1]
        const traceId = messages[i + 1].traceId
        if (traceId && traces[traceId]) exchange.trace = traces[traceId]
        i += 2
      } else {
        const traceId = msg.traceId
        if (traceId && traces[traceId]) exchange.trace = traces[traceId]
        i += 1
      }
      exchanges.push(exchange)
    } else {
      // Non-user message (system role or agent-initiated assistant)
      // For system role: show content in the "user" slot so it's visible
      // For assistant without user: label as system-initiated
      const isSystemRole = msg.role === "system"
      if (isSystemRole && !msg.content) {
        // Skip empty system messages entirely
        i += 1
        continue
      }
      exchanges.push({
        userMessage: {
          role: "user" as const,
          content: isSystemRole ? msg.content : "(系统触发)",
        },
        assistantMessage: isSystemRole ? undefined : msg,
        trace: msg.traceId ? traces[msg.traceId] : undefined,
        isSystemInitiated: true,
      })
      i += 1
    }
  }

  return exchanges
}

// ===== Iteration grouping =====

interface ToolPair {
  call: ExecutionStep
  result?: ExecutionStep
}

interface IterationData {
  iteration: number
  llmCall?: ExecutionStep
  systemSteps: ExecutionStep[]
  thinkings: ExecutionStep[]
  toolPairs: ToolPair[]
  contentSteps: ExecutionStep[]
  errorSteps: ExecutionStep[]
}

function groupStepsByIteration(steps: ExecutionStep[]): IterationData[] {
  const iterMap = new Map<number, IterationData>()
  const pendingToolCalls = new Map<string, { iterIdx: number; pairIdx: number }>()

  const getOrCreate = (iter: number): IterationData => {
    if (!iterMap.has(iter)) {
      iterMap.set(iter, { iteration: iter, systemSteps: [], thinkings: [], toolPairs: [], contentSteps: [], errorSteps: [] })
    }
    return iterMap.get(iter)!
  }

  for (const step of steps) {
    const iter = step.iteration ?? 0
    const data = getOrCreate(iter)

    if (step.type === "llm_call") {
      data.llmCall = step
    } else if (step.type === "thinking") {
      if (step.source === "system") data.systemSteps.push(step)
      else data.thinkings.push(step)
    } else if (step.type === "tool_call") {
      const pairIdx = data.toolPairs.length
      data.toolPairs.push({ call: step })
      if (step.toolCallId) pendingToolCalls.set(step.toolCallId, { iterIdx: iter, pairIdx })
    } else if (step.type === "tool_result") {
      const loc = step.toolCallId ? pendingToolCalls.get(step.toolCallId) : undefined
      if (loc && iterMap.has(loc.iterIdx)) {
        iterMap.get(loc.iterIdx)!.toolPairs[loc.pairIdx].result = step
        if (step.toolCallId) pendingToolCalls.delete(step.toolCallId)
      }
    } else if (step.type === "content") {
      data.contentSteps.push(step)
    } else if (step.type === "error") {
      data.errorSteps.push(step)
    }
  }

  return Array.from(iterMap.values()).sort((a, b) => a.iteration - b.iteration)
}

// ===== Detail panel (expandable code block) =====

function DetailBlock({ children }: { children: React.ReactNode }) {
  return (
    <div className="mt-1.5 rounded-md border border-slate-200 bg-slate-50/80 p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-48 overflow-y-auto">
      {children}
    </div>
  )
}

function LLMIOInline({
  traceId,
  llmIORef,
}: {
  traceId: string
  llmIORef: string
}) {
  const [expanded, setExpanded] = useState(false)
  const { data, isLoading, error } = useQuery({
    queryKey: ["llm-io-inline", traceId, llmIORef],
    queryFn: () => tracesApi.getLLMIO(traceId, llmIORef),
    enabled: expanded,
    staleTime: Infinity,
  })
  const payload = data?.data as Record<string, unknown> | undefined

  return (
    <div className="mb-2 rounded-md border border-indigo-100 bg-indigo-50/40 overflow-hidden">
      <button
        className="w-full flex items-center gap-1.5 px-2.5 py-1.5 text-left hover:bg-indigo-100/50 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        {expanded
          ? <ChevronDown className="h-3 w-3 text-indigo-500" />
          : <ChevronRight className="h-3 w-3 text-indigo-500" />}
        <span className="text-[10px] font-semibold uppercase tracking-wider text-indigo-700">Model I/O</span>
        <span className="text-[10px] font-mono text-indigo-500">{llmIORef}</span>
      </button>

      {expanded && (
        <div className="px-2.5 pb-2.5 space-y-2">
          {isLoading && (
            <div className="space-y-1.5">
              <Skeleton className="h-5 w-24" />
              <Skeleton className="h-20 rounded-md" />
              <Skeleton className="h-5 w-24" />
              <Skeleton className="h-20 rounded-md" />
            </div>
          )}

          {error && <div className="text-[11px] text-red-600">加载失败: {String(error)}</div>}

          {payload && payload.request != null && (
            <div className="rounded-md border border-brand-100 bg-white overflow-hidden">
              <div className="px-2 py-0.5 bg-brand-50 text-[10px] font-semibold text-brand-700 uppercase tracking-wider">
                Input
              </div>
              <pre className="p-2 text-[11px] font-mono text-slate-700 overflow-x-auto whitespace-pre-wrap max-h-56 overflow-y-auto">
                {typeof payload.request === "string"
                  ? payload.request
                  : JSON.stringify(payload.request, null, 2)}
              </pre>
            </div>
          )}

          {payload && payload.response != null && (
            <div className="rounded-md border border-green-100 bg-white overflow-hidden">
              <div className="px-2 py-0.5 bg-green-50 text-[10px] font-semibold text-green-700 uppercase tracking-wider">
                Output
              </div>
              <pre className="p-2 text-[11px] font-mono text-slate-700 overflow-x-auto whitespace-pre-wrap max-h-56 overflow-y-auto">
                {typeof payload.response === "string"
                  ? payload.response
                  : JSON.stringify(payload.response, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ===== Tool Pair =====

function ToolPairView({ pair }: { pair: ToolPair }) {
  const [expanded, setExpanded] = useState(false)
  const { call, result } = pair
  const success = result ? result.toolSuccess !== false : undefined
  const hasDetail = !!(call.toolInput || result?.toolResult || result?.error)

  return (
    <div className="flex gap-2.5 group/tool">
      <div className="flex flex-col items-center pt-0.5">
        <div className={cn(
          "flex h-5 w-5 items-center justify-center rounded-full flex-shrink-0",
          success === false ? "bg-red-50" : success === true ? "bg-green-50" : "bg-brand-50"
        )}>
          {success === false
            ? <XCircle className="h-3 w-3 text-red-600" />
            : success === true
              ? <CheckCircle2 className="h-3 w-3 text-green-600" />
              : <Wrench className="h-3 w-3 text-brand-600" />
          }
        </div>
        <div className="w-px flex-1 bg-slate-200 mt-0.5" />
      </div>

      <div className="flex-1 pb-2.5 min-w-0">
        <div
          className={cn("flex items-center gap-1.5", hasDetail && "cursor-pointer")}
          onClick={() => hasDetail && setExpanded(!expanded)}
        >
          <span className="text-[10px] font-semibold uppercase tracking-wider text-brand-600">Tool</span>
          <span className="text-[13px] font-medium text-slate-700">{call.toolName}</span>
          {result?.toolDuration != null && (
            <span className="flex items-center gap-0.5 text-[10px] text-slate-400 ml-auto">
              <Clock className="h-2.5 w-2.5" />
              {formatDuration(result.toolDuration)}
            </span>
          )}
          {hasDetail && (
            <span className={cn("text-slate-300 transition-opacity opacity-0 group-hover/tool:opacity-100", !result?.toolDuration && "ml-auto")}>
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </span>
          )}
        </div>

        {expanded && (
          <div className="space-y-1.5">
            {call.toolInput != null && (
              <div className="mt-1.5 rounded-md border border-brand-100 bg-brand-50/40 overflow-hidden">
                <div className="px-2.5 py-0.5 bg-brand-100/50 text-[10px] font-semibold text-brand-700 uppercase tracking-wider">Input</div>
                <div className="p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-40 overflow-y-auto">
                  {typeof call.toolInput === "string" ? call.toolInput : JSON.stringify(call.toolInput, null, 2)}
                </div>
              </div>
            )}
            {(result?.toolResult != null || result?.error) && (
              <div className={cn(
                "rounded-md border overflow-hidden",
                result.toolSuccess === false ? "border-red-100 bg-red-50/40" : "border-green-100 bg-green-50/40"
              )}>
                <div className={cn(
                  "px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
                  result.toolSuccess === false ? "bg-red-100/50 text-red-700" : "bg-green-100/50 text-green-700"
                )}>
                  {result.toolSuccess === false ? "Error" : "Result"}
                </div>
                <div className="p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-40 overflow-y-auto">
                  {result.error
                    ? <span className="text-red-600">{result.error}</span>
                    : typeof result.toolResult === "string"
                      ? result.toolResult
                      : JSON.stringify(result.toolResult, null, 2)
                  }
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// ===== Thinking block =====

function ThinkingView({ step }: { step: ExecutionStep }) {
  const [expanded, setExpanded] = useState(false)
  const isSystem = step.source === "system"
  const text = step.thinking ?? ""
  const isLong = text.length > 120 || text.includes('\n')
  const preview = isLong ? truncate(text.split('\n')[0], 120) : text

  return (
    <div className="flex gap-2.5 group/think">
      <div className="flex flex-col items-center pt-0.5">
        <div className={cn("flex h-5 w-5 items-center justify-center rounded-full flex-shrink-0",
          isSystem ? "bg-purple-50" : "bg-slate-100")}>
          {isSystem
            ? <Cpu className="h-3 w-3 text-purple-500" />
            : <Brain className="h-3 w-3 text-slate-400" />
          }
        </div>
        <div className="w-px flex-1 bg-slate-200 mt-0.5" />
      </div>

      <div className="flex-1 pb-2 min-w-0">
        <div
          className={cn("flex items-start gap-1.5", isLong && "cursor-pointer")}
          onClick={() => isLong && setExpanded(!expanded)}
        >
          <span className={cn("text-[10px] font-semibold uppercase tracking-wider",
            isSystem ? "text-purple-500" : "text-slate-400")}>
            {isSystem ? "System" : "Think"}
          </span>
          {isLong && (
            <span className="ml-auto text-slate-300 opacity-0 group-hover/think:opacity-100 transition-opacity">
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </span>
          )}
        </div>
        <p className="text-[13px] text-slate-600 mt-0.5 leading-relaxed break-words">
          {expanded ? text : preview}
        </p>
        {expanded && isLong && (
          <button onClick={() => setExpanded(false)} className="mt-1 text-[10px] text-slate-400 hover:text-slate-600">
            收起
          </button>
        )}
      </div>
    </div>
  )
}

// ===== LLM I/O Viewer =====

function LLMIOViewer({ traceId, llmIORef, onClose }: { traceId: string; llmIORef: string; onClose: () => void }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["llm-io", traceId, llmIORef],
    queryFn: () => tracesApi.getLLMIO(traceId, llmIORef),
    staleTime: Infinity,
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div
        className="relative bg-white rounded-xl shadow-2xl w-[90vw] max-w-4xl max-h-[85vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-5 py-3 border-b border-slate-200 flex-shrink-0">
          <div className="flex items-center gap-2">
            <FileText className="h-4 w-4 text-brand-600" />
            <h3 className="text-sm font-semibold text-slate-900">LLM I/O</h3>
            <span className="text-xs font-mono text-slate-400">{llmIORef}</span>
          </div>
          <button onClick={onClose} className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-5">
          {isLoading && (
            <div className="space-y-3">
              <Skeleton className="h-6 w-32" />
              <Skeleton className="h-40 rounded-lg" />
              <Skeleton className="h-6 w-32" />
              <Skeleton className="h-40 rounded-lg" />
            </div>
          )}
          {error && (
            <div className="text-sm text-red-600">加载失败: {String(error)}</div>
          )}
          {data?.data && (
            <div className="space-y-4">
              {data.data.request != null && (
                <div className="rounded-lg border border-brand-100 overflow-hidden">
                  <div className="px-3 py-1.5 bg-brand-50 text-xs font-semibold text-brand-700 uppercase tracking-wider">
                    Request
                  </div>
                  <pre className="p-3 text-[11px] font-mono text-slate-700 overflow-x-auto whitespace-pre-wrap max-h-[35vh] overflow-y-auto bg-white">
                    {typeof data.data.request === "string"
                      ? data.data.request
                      : JSON.stringify(data.data.request, null, 2)}
                  </pre>
                </div>
              )}
              {data.data.response != null && (
                <div className="rounded-lg border border-green-100 overflow-hidden">
                  <div className="px-3 py-1.5 bg-green-50 text-xs font-semibold text-green-700 uppercase tracking-wider">
                    Response
                  </div>
                  <pre className="p-3 text-[11px] font-mono text-slate-700 overflow-x-auto whitespace-pre-wrap max-h-[35vh] overflow-y-auto bg-white">
                    {typeof data.data.response === "string"
                      ? data.data.response
                      : JSON.stringify(data.data.response, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// ===== Iteration Group =====

function IterationGroup({ data, traceId, defaultExpanded }: { data: IterationData; traceId?: string; defaultExpanded: boolean }) {
  const [expanded, setExpanded] = useState(defaultExpanded)
  const [llmIOOpen, setLlmIOOpen] = useState(false)
  const { llmCall, systemSteps, thinkings, toolPairs, contentSteps, errorSteps } = data
  const hasErrors = errorSteps.length > 0 || toolPairs.some(p => p.result?.toolSuccess === false)
  const isSystemOnly = data.iteration === 0
  const hasLLMIO = !!llmCall?.llmIORef && !!traceId

  const iterLabel = isSystemOnly ? "初始化" : `Iteration ${data.iteration}`

  const modelShort = llmCall?.model
    ? llmCall.model.replace(/^claude-/, "").replace(/-\d{8}$/, "")
    : null

  return (
    <div className="mb-1.5">
      <div className="flex items-center">
        <button
          onClick={() => setExpanded(!expanded)}
          className={cn(
            "flex items-center gap-1.5 flex-1 py-1 px-2 rounded-md text-left transition-colors",
            expanded ? "bg-slate-100/80" : "hover:bg-slate-100/60",
            hasErrors && "text-red-700"
          )}
        >
          {expanded ? <ChevronDown className="h-3 w-3 text-slate-400 flex-shrink-0" /> : <ChevronRight className="h-3 w-3 text-slate-400 flex-shrink-0" />}
          <span className={cn(
            "text-[11px] font-semibold",
            isSystemOnly ? "text-purple-600" : "text-slate-600"
          )}>
            {iterLabel}
          </span>

          {llmCall && (
            <>
              {modelShort && (
                <span className="text-[10px] text-slate-400 font-mono ml-1">{modelShort}</span>
              )}
              <span className="flex items-center gap-0.5 text-[10px] text-slate-400">
                <Zap className="h-2.5 w-2.5" />
                {llmCall.inputTokens ?? "?"}↑{llmCall.outputTokens ?? "?"}↓
              </span>
              {llmCall.durationMs != null && (
                <span className="flex items-center gap-0.5 text-[10px] text-slate-400">
                  <Clock className="h-2.5 w-2.5" />
                  {formatDuration(llmCall.durationMs)}
                </span>
              )}
            </>
          )}

          {toolPairs.length > 0 && (
            <span className="text-[10px] text-slate-400 ml-1">{toolPairs.length} 工具</span>
          )}
          {hasErrors && <span className="text-[10px] text-red-500 ml-1">⚠</span>}
          {llmCall?.costUsd != null && llmCall.costUsd > 0 && (
            <span className="ml-auto text-[10px] text-slate-400">{formatCost(llmCall.costUsd)}</span>
          )}
        </button>

        {hasLLMIO && (
          <button
            onClick={() => setLlmIOOpen(true)}
            className="flex items-center gap-0.5 px-1.5 py-0.5 ml-1 rounded text-[10px] text-slate-400 hover:text-brand-600 hover:bg-brand-50 transition-colors"
            title="查看完整 LLM 请求/响应"
          >
            <FileText className="h-3 w-3" />
            <span>I/O</span>
          </button>
        )}
      </div>

      {expanded && (
        <div className="ml-3 pl-2.5 border-l-2 border-slate-200 mt-0.5">
          {hasLLMIO && <LLMIOInline traceId={traceId!} llmIORef={llmCall!.llmIORef!} />}
          {systemSteps.map((s, i) => <ThinkingView key={i} step={s} />)}
          {thinkings.map((s, i) => <ThinkingView key={i} step={s} />)}
          {toolPairs.map((pair, i) => <ToolPairView key={i} pair={pair} />)}
          {contentSteps.map((s, i) => (
            <div key={i} className="flex gap-2.5">
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
            <div key={i} className="flex gap-2.5">
              <div className="flex flex-col items-center pt-0.5">
                <div className="flex h-5 w-5 items-center justify-center rounded-full bg-red-50 flex-shrink-0">
                  <XCircle className="h-3 w-3 text-red-600" />
                </div>
              </div>
              <div className="flex-1 pb-2 min-w-0">
                <span className="text-[10px] font-semibold uppercase tracking-wider text-red-600">Error</span>
                <DetailBlock><span className="text-red-600">{s.error}</span></DetailBlock>
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

// ===== Single Exchange Card =====

function ExchangeCard({ exchange, isSessionProcessing }: { exchange: MessageExchange; isSessionProcessing?: boolean }) {
  const [traceOpen, setTraceOpen] = useState(false)
  const trace = exchange.trace
  const steps = trace?.steps ?? []
  const hasTrace = steps.length > 0
  // Only treat as "running" if session is still actively processing — prevents
  // stale "(生成中...)" on traces that never received a "done" event (e.g. crashes)
  const isRunning = trace?.status === "running" && !!isSessionProcessing

  useEffect(() => {
    if (isRunning) setTraceOpen(true)
  }, [isRunning])

  const iterGroups = useMemo(() => groupStepsByIteration(steps), [steps])
  const maxIteration = iterGroups.filter(g => g.iteration > 0).length
  const thinkSteps = steps.filter((s) => s.type === "thinking").length
  const toolCalls = steps.filter((s) => s.type === "tool_call").length
  const errors = steps.filter((s) => s.type === "error" || (s.type === "tool_result" && s.toolSuccess === false)).length
  const duration = trace?.completedAt ? trace.completedAt - trace.startedAt : 0
  // Trace is "stuck" if it reports running but the session is no longer processing
  const isStaleTrace = trace?.status === "running" && !isSessionProcessing

  return (
    <div className="group">
      {/* User message or System-initiated trigger */}
      {exchange.isSystemInitiated ? (
        <div className="flex gap-3 px-5 py-3">
          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-purple-50 flex-shrink-0 mt-0.5">
            <Settings2 className="h-3.5 w-3.5 text-purple-600" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-purple-600">系统</span>
              {exchange.userMessage.createdAt && (
                <span className="text-[11px] text-slate-400">{timeAgo(exchange.userMessage.createdAt)}</span>
              )}
            </div>
            <p className="text-sm text-slate-700 mt-0.5 leading-relaxed whitespace-pre-wrap">
              {exchange.userMessage.content || "(系统触发)"}
            </p>
          </div>
        </div>
      ) : (
        <div className="flex gap-3 px-5 py-3">
          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-brand-50 flex-shrink-0 mt-0.5">
            <User className="h-3.5 w-3.5 text-brand-600" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-slate-500">用户</span>
              {exchange.userMessage.createdAt && (
                <span className="text-[11px] text-slate-400">{timeAgo(exchange.userMessage.createdAt)}</span>
              )}
              {exchange.userMessage.initiator && exchange.userMessage.initiator !== "user" && (
                <Badge variant="outline" className="text-[10px]">{exchange.userMessage.initiator}</Badge>
              )}
            </div>
            <p className="text-sm text-slate-900 mt-0.5 leading-relaxed">{exchange.userMessage.content}</p>
          </div>
        </div>
      )}

      {/* Processing trace (collapsible) */}
      {hasTrace && (
        <div className="mx-5 my-1">
          <button
            onClick={() => setTraceOpen(!traceOpen)}
            className={cn(
              "flex items-center gap-2 w-full px-3 py-1.5 rounded-md text-xs transition-colors cursor-pointer",
              isRunning
                ? "bg-brand-50 text-brand-700 hover:bg-brand-100"
                : isStaleTrace
                  ? "bg-amber-50 text-amber-700 hover:bg-amber-100"
                  : errors > 0
                    ? "bg-red-50 text-red-700 hover:bg-red-100"
                    : "bg-slate-50 text-slate-600 hover:bg-slate-100"
            )}
          >
            {isRunning ? (
              <span className="h-2 w-2 rounded-full bg-brand-500 animate-live-pulse" />
            ) : (
              traceOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />
            )}
            <span className="font-medium">
              {isRunning ? "正在处理..." : isStaleTrace ? "已中断" : "处理过程"}
            </span>
            <span className="text-slate-400">·</span>
            {maxIteration > 0 && <span>{maxIteration} 轮</span>}
            {thinkSteps > 0 && <span>{thinkSteps} 思考</span>}
            {toolCalls > 0 && <span>{toolCalls} 工具</span>}
            {errors > 0 && <span className="text-red-600">{errors} 错误</span>}
            {duration > 0 && <span>{formatDuration(duration)}</span>}
            {trace && trace.totalCostUsd > 0 && <span>{formatCost(trace.totalCostUsd)}</span>}
          </button>

          {traceOpen && (
            <div className="mt-2">
              {iterGroups.map((group) => (
                <IterationGroup
                  key={group.iteration}
                  data={group}
                  traceId={trace?.id}
                  defaultExpanded={
                    isRunning
                      ? group.iteration === iterGroups[iterGroups.length - 1].iteration
                      : group.iteration > 0 && maxIteration <= 3
                  }
                />
              ))}
            </div>
          )}
        </div>
      )}

      {/* Assistant response */}
      {exchange.assistantMessage && (
        <div className="flex gap-3 px-5 py-3">
          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-slate-100 flex-shrink-0 mt-0.5">
            <Bot className="h-3.5 w-3.5 text-slate-600" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-slate-500">助手</span>
              {exchange.assistantMessage.createdAt && (
                <span className="text-[11px] text-slate-400">{timeAgo(exchange.assistantMessage.createdAt)}</span>
              )}
            </div>
            <div className="mt-0.5 text-sm text-slate-800">
              <MarkdownContent content={exchange.assistantMessage.content} />
            </div>
          </div>
        </div>
      )}

      {/* Still processing (no response yet) */}
      {!exchange.assistantMessage && isRunning && (
        <div className="flex gap-3 px-5 py-3">
          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-slate-100 flex-shrink-0 mt-0.5">
            <Bot className="h-3.5 w-3.5 text-slate-400 animate-pulse" />
          </div>
          <div className="flex-1">
            <span className="text-xs text-slate-400">生成中...</span>
          </div>
        </div>
      )}
    </div>
  )
}

// ===== Session Detail Panel (right) =====

function SessionPanel({ sessionId }: { sessionId: string }) {
  const { data: session, isLoading } = useSession(sessionId)
  const isProcessing = session?.executionStatus === "processing"
  const { data: messages = [] } = useSessionMessages(sessionId, {
    refetchInterval: isProcessing ? 1000 : 5000,
  })
  const scrollRef = useRef<HTMLDivElement>(null)

  const traceIds = useMemo(() => {
    return [...new Set(
      messages
        .map((m) => m.traceId)
        .filter(Boolean) as string[]
    )]
  }, [messages])

  const { data: fullTraces } = useQuery({
    queryKey: ["traces", "full", traceIds],
    queryFn: async () => {
      const results = await Promise.all(traceIds.map((tid) => tracesApi.getById(tid)))
      return results.reduce<Record<string, ExecutionTrace>>((acc, res) => {
        if (res.data) acc[res.data.id] = res.data
        return acc
      }, {})
    },
    enabled: traceIds.length > 0,
    refetchInterval: 1000,
  })

  const exchanges = useMemo(() => {
    if (messages.length === 0) return []
    return buildExchanges(messages, fullTraces ?? {})
  }, [messages, fullTraces])

  // Auto-scroll to bottom on new exchanges
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [exchanges.length])

  if (isLoading) {
    return (
      <div className="p-6 space-y-4">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-24 rounded-lg" />
        <Skeleton className="h-24 rounded-lg" />
      </div>
    )
  }

  if (!session) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-slate-400">
        会话未找到
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      {/* Session header */}
      <div className="flex items-center justify-between px-5 py-3 border-b border-slate-200 bg-white flex-shrink-0">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-semibold text-slate-900 truncate">
              {session.channelName || session.title || `会话 ${session.id?.slice(0, 8) || "未知"}`}
            </h2>
            {session.sourceChannel && <ChannelBadge channel={session.sourceChannel} />}
            {session.agentId && (
              <Badge variant="outline" className="text-[10px]">{session.agentId}</Badge>
            )}
            {isProcessing && (
              <span className="h-2 w-2 rounded-full bg-green-500 animate-live-pulse" />
            )}
          </div>
          <p className="text-xs text-slate-400 mt-0.5">
            {messages.length} 条消息 · {exchanges.length} 次交互
          </p>
        </div>
      </div>

      {/* Exchange list */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        {exchanges.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <MessageSquare className="h-8 w-8 text-slate-300 mb-2" />
            <p className="text-sm text-slate-400">暂无消息</p>
          </div>
        ) : (
          <div className="divide-y divide-slate-100 py-2">
            {exchanges.map((exchange, i) => (
              <ExchangeCard key={i} exchange={exchange} isSessionProcessing={isProcessing} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ===== Main Monitor Page =====

export function MonitorPage() {
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null)
  const [search, setSearch] = useState("")
  const { data: sessions, isLoading } = useMonitorSessions()
  const deleteSession = useDeleteSession()

  // Filter sessions
  const filteredSessions = useMemo(() => {
    if (!sessions) return []
    if (!search) return sessions
    const q = search.toLowerCase()
    return sessions.filter((s) =>
      (s.title?.toLowerCase().includes(q))
      || (s.channelName?.toLowerCase().includes(q))
      || (s.agentId?.toLowerCase().includes(q))
      || (s.sourceChannel?.toLowerCase().includes(q))
    )
  }, [sessions, search])

  // Auto-select first session
  useEffect(() => {
    if (!selectedSessionId && sessions && sessions.length > 0) {
      const processing = sessions.find((s) => s.executionStatus === "processing")
      setSelectedSessionId(processing?.id ?? sessions[0].id)
    }
  }, [selectedSessionId, sessions])

  const handleDeleteSession = useCallback((e: React.MouseEvent, sessionId: string) => {
    e.stopPropagation()
    if (!confirm("确定删除此会话？将同时清除数据库记录、执行链路和 Agent 工作目录，不可恢复。")) return
    deleteSession.mutate(sessionId, {
      onSuccess: () => {
        if (selectedSessionId === sessionId) {
          setSelectedSessionId(null)
        }
      },
    })
  }, [deleteSession, selectedSessionId])

  return (
    <div className="flex h-full">
      {/* Left: Session list */}
      <div className="w-80 flex-shrink-0 border-r border-slate-200 bg-white flex flex-col">
        <div className="p-3 border-b border-slate-100 flex-shrink-0">
          <div className="flex items-center justify-between mb-2">
            <h2 className="text-sm font-semibold text-slate-900">会话</h2>
            {sessions && (
              <span className="text-[11px] text-slate-400">{sessions.length} 个</span>
            )}
          </div>
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-400" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索会话..."
              className="h-8 pl-8 text-xs"
            />
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          {isLoading ? (
            <div className="p-3 space-y-2">
              {[1, 2, 3, 4, 5].map((i) => <Skeleton key={i} className="h-14 rounded-md" />)}
            </div>
          ) : filteredSessions.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-center">
              <Activity className="h-6 w-6 text-slate-300 mb-2" />
              <p className="text-xs text-slate-400">暂无会话</p>
            </div>
          ) : (
            <div className="py-1">
              {filteredSessions.map((session) => {
                const isSelected = session.id === selectedSessionId
                const isProcessing = session.executionStatus === "processing"

                return (
                  <div
                    key={session.id}
                    onClick={() => setSelectedSessionId(session.id)}
                    className={cn(
                      "w-full text-left px-3 py-2.5 transition-colors cursor-pointer group/item relative",
                      isSelected
                        ? "bg-brand-50 border-r-2 border-brand-600"
                        : "hover:bg-slate-50"
                    )}
                  >
                    <div className="flex items-center gap-2">
                      {isProcessing && (
                        <span className="h-2 w-2 rounded-full bg-green-500 animate-live-pulse flex-shrink-0" />
                      )}
                      <span className={cn(
                        "text-sm font-medium truncate flex-1",
                        isSelected ? "text-brand-700" : "text-slate-900"
                      )}>
                        {session.channelName || session.title || session.id?.slice(0, 10) || "未知会话"}
                      </span>
                      <button
                        onClick={(e) => handleDeleteSession(e, session.id)}
                        className="opacity-0 group-hover/item:opacity-100 p-1 rounded hover:bg-red-100 text-slate-400 hover:text-red-600 transition-all flex-shrink-0"
                        title="删除会话及相关数据"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <div className="flex items-center gap-2 mt-1">
                      {session.sourceChannel && <ChannelBadge channel={session.sourceChannel} />}
                      {session.executionStatus && <StatusBadge status={session.executionStatus} />}
                      <span className="text-[11px] text-slate-400 ml-auto">
                        {(session.updatedAt || session.createdAt) ? timeAgo(session.updatedAt || session.createdAt) : ""}
                      </span>
                    </div>
                    <div className="flex items-center gap-2 mt-0.5 text-[11px] text-slate-400">
                      <span>{session.agentId || "default"}</span>
                      {session.messageCount > 0 && <span>· {session.messageCount} 条</span>}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>

      {/* Right: Session processing detail */}
      <div className="flex-1 bg-slate-50 flex flex-col min-w-0">
        {selectedSessionId ? (
          <SessionPanel sessionId={selectedSessionId} />
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
    </div>
  )
}
