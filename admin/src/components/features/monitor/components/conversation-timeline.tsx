import { useRef, useEffect, useMemo } from "react"
import { User, Bot, Settings2, MessageSquare } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { MarkdownContent } from "@/components/shared/markdown-content"
import { cn, timeAgo } from "@/lib/utils"
import type { MessageExchange } from "../lib/types"
import { CompactionEvent } from "./compaction-event"
import { ExchangeSkeleton } from "./exchange-skeleton"
import type { CompactionData } from "@/api/types"

interface Props {
  exchanges: MessageExchange[]
  compactions: CompactionData[]
  isProcessing: boolean
  selectedExchangeIndex: number | null
  onSelectExchange: (index: number) => void
  isLoadingMessages: boolean
}

export function ConversationTimeline({
  exchanges, compactions, isProcessing,
  selectedExchangeIndex, onSelectExchange, isLoadingMessages,
}: Props) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const wasAtBottomRef = useRef(true)

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
    wasAtBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 60
  }

  const compactionsByTime = useMemo(() =>
    [...compactions].sort((a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()),
    [compactions]
  )

  if (isLoadingMessages) {
    return (
      <div className="flex-1 overflow-y-auto">
        <ExchangeSkeleton />
        <ExchangeSkeleton />
        <ExchangeSkeleton />
      </div>
    )
  }

  if (exchanges.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-center">
        <MessageSquare className="h-8 w-8 text-slate-300 mb-2" />
        <p className="text-sm text-slate-400">暂无消息</p>
      </div>
    )
  }

  let cIdx = 0

  return (
    <div ref={scrollRef} onScroll={handleScroll} className="flex-1 overflow-y-auto">
      <div className="divide-y divide-slate-100 py-2">
        {exchanges.map((exchange) => {
          const exchangeTime = exchange.userMessage.createdAt
            ? new Date(exchange.userMessage.createdAt).getTime()
            : 0

          const compactionsBeforeThis: CompactionData[] = []
          while (cIdx < compactionsByTime.length) {
            const cTime = new Date(compactionsByTime[cIdx].createdAt).getTime()
            if (cTime < exchangeTime) {
              compactionsBeforeThis.push(compactionsByTime[cIdx])
              cIdx++
            } else break
          }

          const isSelected = selectedExchangeIndex === exchange.exchangeIndex
          const trace = exchange.trace
          const isRunning = trace?.status === "running" && isProcessing
          const isStale = trace?.status === "running" && !isProcessing
          const steps = trace?.steps ?? []
          const toolCalls = steps.filter(s => s.type === "tool_call").length
          const errors = steps.filter(s => s.type === "error" || (s.type === "tool_result" && s.toolSuccess === false)).length

          return (
            <div key={exchange.exchangeIndex}>
              {compactionsBeforeThis.map((c) => (
                <CompactionEvent key={c.id} data={c} />
              ))}

              <div
                className={cn(
                  "cursor-pointer transition-colors",
                  isSelected ? "bg-brand-50/50" : "hover:bg-slate-50/50"
                )}
                onClick={() => onSelectExchange(exchange.exchangeIndex)}
              >
                {/* User message */}
                {exchange.isSystemInitiated ? (
                  <div className="flex gap-3 px-5 py-3">
                    <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-purple-50">
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
                    <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-brand-50">
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

                {/* Trace indicator */}
                {trace && (
                  <div className="mx-5 mb-1">
                    <div className={cn(
                      "flex items-center gap-2 px-3 py-1 rounded-md text-[11px]",
                      isRunning ? "bg-brand-50 text-brand-700"
                        : isStale ? "bg-amber-50 text-amber-700"
                          : errors > 0 ? "bg-red-50 text-red-700"
                            : "bg-slate-50 text-slate-500"
                    )}>
                      {isRunning && <span className="h-1.5 w-1.5 rounded-full bg-brand-500 animate-live-pulse" />}
                      <span className="font-medium">
                        {isRunning ? "正在处理..." : isStale ? "已中断" : "处理完成"}
                      </span>
                      {toolCalls > 0 && <span>{toolCalls} 工具</span>}
                      {errors > 0 && <span className="text-red-600">{errors} 错误</span>}
                    </div>
                  </div>
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
                          <span className="text-[11px] text-slate-400">{timeAgo(exchange.assistantMessage.createdAt)}</span>
                        )}
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
  )
}
