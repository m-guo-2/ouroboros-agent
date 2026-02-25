import { useState, useEffect, useRef, useMemo, useCallback } from "react"
import {
  Activity, MessageSquare,
  User, Bot, Brain, Wrench, CheckCircle2, XCircle,
  Clock, ChevronDown, ChevronRight, Search, Trash2,
  Cpu, Settings2, Eye
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/shared/status-badge"
import { ChannelBadge } from "@/components/shared/channel-badge"
import { MarkdownContent } from "@/components/shared/markdown-content"
import { useMonitorSessions } from "@/hooks/use-monitor"
import { useSession, useDeleteSession } from "@/hooks/use-sessions"
import { useQuery } from "@tanstack/react-query"
import { tracesApi } from "@/api/traces"
import { cn, timeAgo, formatDuration, formatCost, truncate } from "@/lib/utils"
import type { AgentMessage, ExecutionTrace, ExecutionStep } from "@/api/types"

// ===== Message Exchange: user msg → trace → assistant response =====

interface MessageExchange {
  userMessage: AgentMessage
  trace?: ExecutionTrace
  assistantMessage?: AgentMessage
  /** true when triggered by system/agent, not a real user message */
  isSystemInitiated?: boolean
}

function buildExchanges(
  messages: AgentMessage[],
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

// ===== Trace Step =====

function TraceStep({ step }: { step: ExecutionStep }) {
  const [expanded, setExpanded] = useState(false)

  const config = {
    thinking: { 
      icon: step.source === "system" ? Cpu : Brain, 
      color: step.source === "system" ? "text-purple-600" : "text-slate-500", 
      bg: step.source === "system" ? "bg-purple-50" : "bg-slate-100", 
      label: step.source === "system" ? "System" : "Think" 
    },
    tool_call: { icon: Wrench, color: "text-brand-600", bg: "bg-brand-50", label: "Act" },
    tool_result: {
      icon: step.toolSuccess === false ? XCircle : CheckCircle2,
      color: step.toolSuccess === false ? "text-red-600" : "text-green-600",
      bg: step.toolSuccess === false ? "bg-red-50" : "bg-green-50",
      label: "Observe",
    },
    content: { icon: CheckCircle2, color: "text-green-600", bg: "bg-green-50", label: "Result" },
    error: { icon: XCircle, color: "text-red-600", bg: "bg-red-50", label: "Error" },
    model_io: { icon: Eye, color: "text-sky-600", bg: "bg-sky-50", label: "Model I/O" },
  }[step.type] ?? { icon: Activity, color: "text-slate-500", bg: "bg-slate-100", label: step.type }

  const Icon = config.icon
  const hasDetail = !!(step.thinking || step.toolInput || step.toolResult || step.error || step.modelInput || step.modelOutput)

  const isMultiLine = step.thinking?.includes('\n')

  const modelOutputData = step.modelOutput as Record<string, unknown> | undefined
  const modelOutputSummary = step.type === "model_io" && modelOutputData
    ? `in=${(modelOutputData.inputTokens as number) ?? "?"} out=${(modelOutputData.outputTokens as number) ?? "?"}${modelOutputData.stopReason ? ` · ${modelOutputData.stopReason}` : ""}`
    : ""

  const summaryText = step.type === "model_io"
    ? modelOutputSummary || "LLM 调用"
    : step.thinking
      ? (isMultiLine ? step.thinking.split('\n')[0] : truncate(step.thinking, 100))
      : step.toolName
        ? `${step.toolName}${step.toolDuration ? ` · ${formatDuration(step.toolDuration)}` : ""}`
        : step.content
          ? truncate(step.content, 100)
          : step.error ?? ""

  return (
    <div className="flex gap-2.5 group/step">
      <div className="flex flex-col items-center pt-0.5">
        <div className={cn("flex h-5 w-5 items-center justify-center rounded-full flex-shrink-0", config.bg)}>
          <Icon className={cn("h-3 w-3", config.color)} />
        </div>
        <div className="w-px flex-1 bg-slate-200 mt-0.5" />
      </div>

      <div className="flex-1 pb-2.5 min-w-0">
        <div
          className={cn("flex items-start gap-1.5", hasDetail && "cursor-pointer")}
          onClick={() => hasDetail && setExpanded(!expanded)}
        >
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1.5">
              <span className={cn("text-[10px] font-semibold uppercase tracking-wider", config.color)}>
                {config.label}
              </span>
              {step.toolDuration != null && (
                <span className="flex items-center gap-0.5 text-[10px] text-slate-400">
                  <Clock className="h-2.5 w-2.5" />
                  {formatDuration(step.toolDuration)}
                </span>
              )}
            </div>
            <p className="text-[13px] text-slate-600 mt-0.5 break-words leading-relaxed">{summaryText}</p>
          </div>
          {hasDetail && (
            <span className="mt-0.5 text-slate-300 opacity-0 group-hover/step:opacity-100 transition-opacity">
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </span>
          )}
        </div>

        {expanded && step.type === "model_io" ? (
          <div className="mt-1.5 space-y-1.5">
            {step.modelInput && (
              <div className="rounded-md border border-sky-100 bg-sky-50/60 overflow-hidden">
                <div className="px-2.5 py-1 bg-sky-100/60 text-[10px] font-semibold text-sky-700 uppercase tracking-wider">Input</div>
                <div className="p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-48 overflow-y-auto">
                  {JSON.stringify(step.modelInput, null, 2)}
                </div>
              </div>
            )}
            {step.modelOutput && (
              <div className="rounded-md border border-slate-200 bg-slate-50/80 overflow-hidden">
                <div className="px-2.5 py-1 bg-slate-100/80 text-[10px] font-semibold text-slate-500 uppercase tracking-wider">Output</div>
                <div className="p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-48 overflow-y-auto">
                  {JSON.stringify(step.modelOutput, null, 2)}
                </div>
              </div>
            )}
          </div>
        ) : expanded ? (
          <div className="mt-1.5 rounded-md border border-slate-200 bg-slate-50/80 p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-48 overflow-y-auto">
            {step.thinking && String(step.thinking)}
            {step.toolInput ? JSON.stringify(step.toolInput, null, 2) : null}
            {step.toolResult ? (typeof step.toolResult === "string" ? step.toolResult : JSON.stringify(step.toolResult, null, 2)) : null}
            {step.error && <span className="text-red-600">{step.error}</span>}
          </div>
        ) : null}
      </div>
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

  const toolCalls = steps.filter((s) => s.type === "tool_call").length
  const thinkSteps = steps.filter((s) => s.type === "thinking" && s.source !== "system").length
  const errors = steps.filter((s) => s.type === "error" || (s.type === "tool_result" && s.toolSuccess === false)).length
  const maxIteration = Math.max(0, ...steps.map(s => s.iteration || 0))
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
              {exchange.userMessage.timestamp && (
                <span className="text-[11px] text-slate-400">{timeAgo(exchange.userMessage.timestamp)}</span>
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
              {exchange.userMessage.timestamp && (
                <span className="text-[11px] text-slate-400">{timeAgo(exchange.userMessage.timestamp)}</span>
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
            <div className="mt-2 ml-2 pl-3 border-l-2 border-slate-200">
              {(() => {
                let lastIteration = 0
                return steps.map((step, i) => {
                  const showHeader = step.iteration > 0 && step.iteration !== lastIteration && step.source !== "system"
                  if (step.iteration > 0) lastIteration = step.iteration
                  return (
                    <div key={i}>
                      {showHeader && (
                        <div className="flex items-center gap-2 mb-1.5 mt-1">
                          <span className="text-[10px] font-semibold text-slate-400 uppercase tracking-wider">
                            Iteration {step.iteration}
                          </span>
                          <div className="flex-1 h-px bg-slate-200" />
                        </div>
                      )}
                      <TraceStep step={step} />
                    </div>
                  )
                })
              })()}
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
              {exchange.assistantMessage.timestamp && (
                <span className="text-[11px] text-slate-400">{timeAgo(exchange.assistantMessage.timestamp)}</span>
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
  const scrollRef = useRef<HTMLDivElement>(null)

  // Is this session actively processing?
  const isProcessing = session?.executionStatus === "processing"

  // Fetch traces for messages
  const traceIds = useMemo(() => {
    return [...new Set(
      (session?.messages ?? [])
        .map((m) => m.traceId)
        .filter(Boolean) as string[]
    )]
  }, [session?.messages])

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
    // Poll faster when processing
    refetchInterval: isProcessing ? 2000 : false,
  })

  const exchanges = useMemo(() => {
    if (!session) return []
    return buildExchanges(session.messages, fullTraces ?? {})
  }, [session, fullTraces])

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
              {session.channelName || session.title || `会话 ${session.id.slice(0, 8)}`}
            </h2>
            {session.sourceChannel && <ChannelBadge channel={session.sourceChannel} />}
            {session.agentDisplayName && (
              <Badge variant="brand">{session.agentDisplayName}</Badge>
            )}
            {isProcessing && (
              <span className="h-2 w-2 rounded-full bg-green-500 animate-live-pulse" />
            )}
          </div>
          <p className="text-xs text-slate-400 mt-0.5">
            {session.messages.length} 条消息 · {exchanges.length} 次交互
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
      || (s.agentDisplayName?.toLowerCase().includes(q))
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
                        {session.channelName || session.title || session.id.slice(0, 10)}
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
                        {session.updatedAt ? timeAgo(session.updatedAt) : ""}
                      </span>
                    </div>
                    <div className="flex items-center gap-2 mt-0.5 text-[11px] text-slate-400">
                      <span>{session.agentDisplayName ?? "default"}</span>
                      <span>· {session.messageCount} 条</span>
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
