import { useRef, useEffect, useMemo, useState } from "react"
import { useTimeAgoTick } from "@/hooks/use-time-ago-tick"
import { Zap, Bot, MessageSquare, Copy, ArrowDown, CheckCircle2, Circle, XCircle } from "lucide-react"
import { MarkdownContent } from "@/components/shared/markdown-content"
import { cn, timeAgo, absoluteTime, copyToClipboard } from "@/lib/utils"
import type { MessageExchange } from "../lib/types"
import { CompactionEvent } from "./compaction-event"
import { ExchangeSkeleton } from "./exchange-skeleton"
import type { CompactionData, ExecutionTrace, MessageLifecycleEvent } from "@/api/types"

interface Props {
  exchanges: MessageExchange[]
  compactions: CompactionData[]
  isProcessing: boolean
  activeTraceId?: string
  selectedTrace?: ExecutionTrace | null
  lifecycleEvents?: MessageLifecycleEvent[]
  selectedExchangeIndex: number | null
  onSelectExchange: (index: number) => void
  isLoadingMessages: boolean
  hasMoreMessages?: boolean
  onLoadMoreMessages?: () => void
  isLoadingMoreMessages?: boolean
}

export function ConversationTimeline({
  exchanges, compactions, isProcessing,
  activeTraceId, selectedTrace, selectedExchangeIndex, lifecycleEvents = [], onSelectExchange, isLoadingMessages,
  hasMoreMessages, onLoadMoreMessages, isLoadingMoreMessages,
}: Props) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const wasAtBottomRef = useRef(true)
  const [showScrollBtn, setShowScrollBtn] = useState(false)
  const timeAgoTick = useTimeAgoTick()

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    if (wasAtBottomRef.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [exchanges.length])

  const handleScroll = () => {
    const el = scrollRef.current
    if (!el) return
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    wasAtBottomRef.current = distanceFromBottom < 60
    setShowScrollBtn(distanceFromBottom > 200)
  }

  const compactionsByTime = useMemo(() =>
    [...compactions].sort((a, b) => a.createdAt - b.createdAt),
    [compactions]
  )

  const lifecycleByMessage = useMemo(() => {
    const map = new Map<number, MessageLifecycleEvent[]>()
    for (const event of lifecycleEvents) {
      if (!event.messageId) continue
      const items = map.get(event.messageId) ?? []
      items.push(event)
      map.set(event.messageId, items)
    }
    return map
  }, [lifecycleEvents])

  if (isLoadingMessages) {
    return (
      <div className="flex-1 overflow-y-auto" data-time-ago-tick={timeAgoTick}>
        <ExchangeSkeleton />
        <ExchangeSkeleton />
        <ExchangeSkeleton />
      </div>
    )
  }

  if (exchanges.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-center" data-time-ago-tick={timeAgoTick}>
        <MessageSquare className="h-8 w-8 text-slate-300 mb-2" />
        <p className="text-sm text-slate-400">暂无消息</p>
      </div>
    )
  }

  let cIdx = 0

  return (
    <div className="relative flex-1 overflow-hidden">
    <div ref={scrollRef} onScroll={handleScroll} className="h-full overflow-y-auto" data-time-ago-tick={timeAgoTick}>
      {hasMoreMessages && (
        <div className="flex justify-center py-3 border-b border-slate-100">
          <button
            onClick={onLoadMoreMessages}
            disabled={isLoadingMoreMessages}
            className="text-xs text-slate-400 hover:text-slate-600 transition-colors disabled:opacity-40"
          >
            {isLoadingMoreMessages ? "加载中..." : "加载更早消息"}
          </button>
        </div>
      )}
      <div className="divide-y divide-slate-100 py-2">
        {exchanges.map((exchange) => {
          const exchangeTime = exchange.userMessage.createdAt ?? 0

          const compactionsBeforeThis: CompactionData[] = []
          while (cIdx < compactionsByTime.length) {
            const cTime = compactionsByTime[cIdx].createdAt
            if (cTime < exchangeTime) {
              compactionsBeforeThis.push(compactionsByTime[cIdx])
              cIdx++
            } else break
          }

          const isSelected = selectedExchangeIndex === exchange.exchangeIndex
          const trace = isSelected ? selectedTrace ?? null : null
          const isRunning = !!exchange.traceId && exchange.traceId === activeTraceId && isProcessing
          const steps = trace?.steps ?? []
          const toolCalls = steps.filter(s => s.type === "tool_call").length
          const errors = steps.filter(s => s.type === "error" || (s.type === "tool_result" && s.toolSuccess === false)).length
          const messageLifecycle = exchange.userMessage.id ? lifecycleByMessage.get(exchange.userMessage.id) ?? [] : []

          const initiator = exchange.userMessage.initiator
          const initiatorStyle = !initiator || initiator === "user"
            ? { label: "用户消息", className: "text-brand-600", bgClass: "bg-brand-50" }
            : initiator === "system" || exchange.isSystemInitiated
            ? { label: "系统触发", className: "text-slate-500", bgClass: "bg-slate-100" }
            : initiator === "scheduled" || initiator === "delayed_task"
            ? { label: "定时任务", className: "text-amber-600", bgClass: "bg-amber-50" }
            : { label: initiator, className: "text-slate-500", bgClass: "bg-slate-100" }

          return (
            <div key={exchange.exchangeIndex}>
              {compactionsBeforeThis.map((c) => (
                <CompactionEvent key={c.id} data={c} />
              ))}

              <div
                className={cn(
                  "group/exchange cursor-pointer transition-colors",
                  isSelected ? "bg-brand-50/50" : "hover:bg-slate-50/50"
                )}
                onClick={() => onSelectExchange(exchange.exchangeIndex)}
              >
                {/* User message */}
                <div className="flex gap-3 px-5 py-3">
                  <div className={cn("mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full", initiatorStyle.bgClass)}>
                    <Zap className={cn("h-3.5 w-3.5", initiatorStyle.className)} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className={cn("text-xs font-medium", initiatorStyle.className)}>{initiatorStyle.label}</span>
                      {exchange.userMessage.createdAt && (
                        <span className="text-[11px] text-slate-400" title={absoluteTime(exchange.userMessage.createdAt)}>{timeAgo(exchange.userMessage.createdAt)}</span>
                      )}
                      <button
                        onClick={(e) => { e.stopPropagation(); copyToClipboard(exchange.userMessage.content || "") }}
                        className="opacity-0 group-hover/exchange:opacity-100 p-1 rounded text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition-all shrink-0"
                        title="复制"
                      >
                        <Copy className="h-3 w-3" />
                      </button>
                    </div>
                    <p className="text-sm text-slate-900 mt-0.5 leading-relaxed whitespace-pre-wrap">
                      {exchange.userMessage.content || "(无内容)"}
                    </p>
                  </div>
                </div>

                {/* Trace indicator */}
                {exchange.traceId && (
                  <div className="mx-5 mb-1">
                    <div className={cn(
                      "flex items-center gap-2 px-3 py-1 rounded-md text-[11px]",
                      isRunning ? "bg-brand-50 text-brand-700"
                        : trace && errors > 0 ? "bg-red-50 text-red-700"
                          : trace ? "bg-slate-50 text-slate-500"
                            : "bg-slate-50 text-slate-500"
                    )}>
                      {isRunning && <span className="h-1.5 w-1.5 rounded-full bg-brand-500 animate-live-pulse" />}
                      <span className="font-medium">
                        {isRunning ? "正在处理..." : trace ? "查看中" : "查看决策详情"}
                      </span>
                      {trace && toolCalls > 0 && <span>{toolCalls} 工具</span>}
                      {trace && errors > 0 && <span className="text-red-600">{errors} 错误</span>}
                    </div>
                  </div>
                )}

                {messageLifecycle.length > 0 && (
                  <LifecycleStatusBar events={messageLifecycle} />
                )}

                {/* Assistant message */}
                {exchange.assistantMessage && (
                  <div className="flex gap-3 px-5 py-3">
                    <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-100">
                      <Bot className="h-3.5 w-3.5 text-slate-600" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-medium text-slate-500">助手</span>
                        {exchange.assistantMessage.createdAt && (
                          <span className="text-[11px] text-slate-400" title={absoluteTime(exchange.assistantMessage.createdAt)}>{timeAgo(exchange.assistantMessage.createdAt)}</span>
                        )}
                        <button
                          onClick={(e) => { e.stopPropagation(); copyToClipboard(exchange.assistantMessage!.content || "") }}
                          className="opacity-0 group-hover/exchange:opacity-100 p-1 rounded text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition-all shrink-0"
                          title="复制"
                        >
                          <Copy className="h-3 w-3" />
                        </button>
                      </div>
                      <div className="mt-0.5 text-sm text-slate-800">
                        <MarkdownContent content={exchange.assistantMessage.content} />
                      </div>
                    </div>
                  </div>
                )}

                {/* Processing placeholder */}
                {!exchange.assistantMessage && isRunning && (
                  <div className="flex gap-3 px-5 py-3">
                    <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-100">
                      <Bot className="h-3.5 w-3.5 text-slate-400 animate-pulse" />
                    </div>
                    <div className="flex-1"><span className="text-xs text-slate-400">生成中...</span></div>
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
    {showScrollBtn && (
      <button
        onClick={() => { scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" }) }}
        className="absolute bottom-4 right-4 flex items-center justify-center h-8 w-8 rounded-full bg-white shadow-md border border-slate-200 text-slate-500 hover:text-slate-700 transition-colors z-10"
        title="滚动到底部"
      >
        <ArrowDown className="h-4 w-4" />
      </button>
    )}
    </div>
  )
}

const STATUS_STEPS = [
  { stage: "message_saved", label: "入库" },
  { stage: "session_event_appended", label: "入队" },
  { stage: "event_drained", label: "消费" },
  { stage: "context_built", label: "上下文" },
  { stage: "processed_completed", label: "完成" },
]

function LifecycleStatusBar({ events }: { events: MessageLifecycleEvent[] }) {
  const byStage = new Map<string, MessageLifecycleEvent>()
  for (const event of events) byStage.set(event.stage, event)
  const done = byStage.get("processed_completed")
  const outcomeLabel = done?.outcome === "replied"
    ? "已回复"
    : done?.outcome === "no_reply"
    ? "未回复"
    : done?.outcome === "send_failed"
    ? "发送失败"
    : done?.outcome || ""

  return (
    <div className="mx-5 mb-2 flex flex-wrap items-center gap-1.5 text-[11px] text-slate-500">
      {STATUS_STEPS.map((step) => {
        const event = byStage.get(step.stage)
        const failed = event?.status === "failed"
        const Icon = failed ? XCircle : event ? CheckCircle2 : Circle
        return (
          <span
            key={step.stage}
            className={cn(
              "inline-flex items-center gap-1 rounded-md border px-2 py-1",
              failed
                ? "border-red-200 bg-red-50 text-red-700"
                : event
                ? "border-emerald-200 bg-emerald-50 text-emerald-700"
                : "border-slate-200 bg-white text-slate-400"
            )}
          >
            <Icon className="h-3 w-3" />
            {step.stage === "processed_completed" && outcomeLabel ? outcomeLabel : step.label}
          </span>
        )
      })}
    </div>
  )
}
