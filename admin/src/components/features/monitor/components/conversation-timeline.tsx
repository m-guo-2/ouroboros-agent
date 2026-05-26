import { useRef, useEffect, useMemo, useState } from "react"
import { useTimeAgoTick } from "@/hooks/use-time-ago-tick"
import { Zap, Bot, MessageSquare, Copy, ArrowDown, Inbox, Timer, CornerDownRight, ArrowRight, MessageCircleReply } from "lucide-react"
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
      <div className="flex h-full flex-col overflow-y-auto" data-time-ago-tick={timeAgoTick}>
        <ExchangeSkeleton />
        <ExchangeSkeleton />
        <ExchangeSkeleton />
      </div>
    )
  }

  if (exchanges.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center text-center" data-time-ago-tick={timeAgoTick}>
        <MessageSquare className="h-8 w-8 text-slate-300 mb-2" />
        <p className="text-sm text-slate-400">暂无消息</p>
      </div>
    )
  }

  let cIdx = 0

  return (
    <div className="relative flex h-full flex-col overflow-hidden">
      <div className="flex h-14 shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4">
        <div className="flex items-center gap-2">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-950 text-white">
            <Inbox className="h-4 w-4" />
          </span>
          <div>
            <div className="text-sm font-semibold text-slate-900">执行入口</div>
            <div className="text-[11px] text-slate-400">选择一条消息，右侧查看完整执行过程</div>
          </div>
        </div>
        <span className="text-[12px] text-slate-400">{exchanges.length} 次交互</span>
      </div>
    <div ref={scrollRef} onScroll={handleScroll} className="min-h-0 flex-1 overflow-y-auto bg-slate-50/60" data-time-ago-tick={timeAgoTick}>
      {hasMoreMessages && (
        <div className="flex justify-center py-3 border-b border-slate-100 bg-white">
          <button
            onClick={onLoadMoreMessages}
            disabled={isLoadingMoreMessages}
            className="text-xs text-slate-400 hover:text-slate-600 transition-colors disabled:opacity-40"
          >
            {isLoadingMoreMessages ? "加载中..." : "加载更早消息"}
          </button>
        </div>
      )}
      <div className="space-y-3 p-3">
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
          const outcome = resolveExchangeOutcome(exchange, messageLifecycle, isRunning)

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
                  "group/exchange cursor-pointer overflow-hidden rounded-xl border bg-white transition-all",
                  outcomeAccent(outcome),
                  isSelected ? "border-slate-900 shadow-md ring-2 ring-slate-300" : "border-slate-200 hover:border-slate-400 hover:shadow-sm"
                )}
                onClick={() => onSelectExchange(exchange.exchangeIndex)}
              >
                <div className="grid grid-cols-[116px_minmax(0,1fr)]">
                  <div className="border-r border-slate-100 bg-slate-50 px-3 py-3">
                    <OutcomeBadge outcome={outcome} hasLifecycle={messageLifecycle.length > 0} />
                    <div className="mt-3 space-y-1 text-[11px] text-slate-500">
                      <div className="flex items-center gap-1.5">
                        <Timer className="h-3 w-3" />
                        <span>#{exchange.exchangeIndex + 1}</span>
                      </div>
                      {exchange.userMessage.createdAt && (
                        <div title={absoluteTime(exchange.userMessage.createdAt)}>{timeAgo(exchange.userMessage.createdAt)}</div>
                      )}
                      <div>{toolCalls} 工具 · {errors} 错误</div>
                    </div>
                  </div>

                  <div className="min-w-0">
                    <div className="flex items-center justify-between gap-3 border-b border-slate-100 px-4 py-3">
                      <div className="flex min-w-0 items-center gap-2">
                        <span className={cn("flex h-7 w-7 shrink-0 items-center justify-center rounded-md", initiatorStyle.bgClass)}>
                          <Zap className={cn("h-3.5 w-3.5", initiatorStyle.className)} />
                        </span>
                        <span className={cn("text-xs font-semibold", initiatorStyle.className)}>{initiatorStyle.label}</span>
                        {exchange.traceId && <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium text-slate-500">trace</span>}
                      </div>
                      <button
                        onClick={(e) => { e.stopPropagation(); copyToClipboard(exchange.userMessage.content || "") }}
                        className="opacity-0 group-hover/exchange:opacity-100 p-1 rounded text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition-all shrink-0"
                        title="复制收到内容"
                      >
                        <Copy className="h-3 w-3" />
                      </button>
                    </div>

                    <div className="px-4 py-3">
                      <p className="line-clamp-4 text-sm leading-relaxed text-slate-950 whitespace-pre-wrap">
                      {exchange.userMessage.content || "(无内容)"}
                      </p>
                    </div>

                    <div className="border-t border-slate-100 bg-slate-50 px-4 py-3">
                      {exchange.assistantMessage ? (
                        <div>
                          <div className="mb-1.5 flex items-center gap-2">
                            <MessageCircleReply className="h-3.5 w-3.5 text-emerald-600" />
                            <span className="text-[11px] font-semibold text-emerald-700">已产生回复</span>
                            {exchange.assistantMessage.createdAt && (
                              <span className="text-[11px] text-slate-400" title={absoluteTime(exchange.assistantMessage.createdAt)}>{timeAgo(exchange.assistantMessage.createdAt)}</span>
                            )}
                            <button
                              onClick={(e) => { e.stopPropagation(); copyToClipboard(exchange.assistantMessage!.content || "") }}
                              className="opacity-0 group-hover/exchange:opacity-100 p-1 rounded text-slate-400 hover:text-slate-600 hover:bg-white transition-all shrink-0"
                              title="复制回复"
                            >
                              <Copy className="h-3 w-3" />
                            </button>
                          </div>
                          <div className="line-clamp-3 text-sm text-slate-700">
                            <MarkdownContent content={exchange.assistantMessage.content} />
                          </div>
                        </div>
                      ) : (
                        <div className={cn(
                          "flex items-center gap-2 text-[12px] font-medium",
                          isRunning ? "text-brand-700" : "text-amber-700"
                        )}>
                          <Bot className={cn("h-3.5 w-3.5", isRunning && "animate-pulse")} />
                          {isRunning ? "正在生成回复" : "没有助手回复，点击查看原因"}
                        </div>
                      )}
                      {exchange.traceId && (
                        <div className="mt-2 flex items-center gap-2 text-[11px] text-slate-500">
                          <CornerDownRight className="h-3.5 w-3.5" />
                            <span>{isSelected ? "右侧正在查看执行过程" : "点击查看执行过程"}</span>
                          <ArrowRight className="h-3 w-3" />
                        </div>
                      )}
                    </div>
                  </div>
                </div>
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

type ExchangeOutcome = "running" | "replied" | "no_reply" | "send_failed" | "failed" | "unknown"

function resolveExchangeOutcome(exchange: MessageExchange, events: MessageLifecycleEvent[], isRunning: boolean): ExchangeOutcome {
  if (isRunning) return "running"
  const failed = events.find((event) => event.status === "failed")
  if (failed) return failed.outcome === "send_failed" ? "send_failed" : "failed"
  const completed = [...events].reverse().find((event) => event.stage === "processed_completed")
  if (completed?.outcome === "replied") return "replied"
  if (completed?.outcome === "no_reply") return "no_reply"
  if (completed?.outcome === "send_failed") return "send_failed"
  if (exchange.assistantMessage) return "replied"
  return "unknown"
}

function OutcomeBadge({ outcome, hasLifecycle }: { outcome: ExchangeOutcome; hasLifecycle: boolean }) {
  const config = {
    running: ["处理中", "border-brand-200 bg-brand-50 text-brand-700"],
    replied: ["已回复", "border-emerald-200 bg-emerald-50 text-emerald-700"],
    no_reply: ["未回复", "border-amber-200 bg-amber-50 text-amber-700"],
    send_failed: ["发送失败", "border-red-200 bg-red-50 text-red-700"],
    failed: ["处理失败", "border-red-200 bg-red-50 text-red-700"],
    unknown: [hasLifecycle ? "未完成" : "无消息流", "border-slate-200 bg-slate-50 text-slate-500"],
  } satisfies Record<ExchangeOutcome, [string, string]>

  const [label, className] = config[outcome]
  return (
    <span className={cn("inline-flex shrink-0 rounded-full border px-2 py-1 text-[11px] font-semibold", className)}>
      {label}
    </span>
  )
}

function outcomeAccent(outcome: ExchangeOutcome) {
  if (outcome === "replied") return "border-l-4 border-l-emerald-500"
  if (outcome === "no_reply" || outcome === "running") return "border-l-4 border-l-amber-500"
  if (outcome === "send_failed" || outcome === "failed") return "border-l-4 border-l-red-500"
  return "border-l-4 border-l-slate-300"
}
