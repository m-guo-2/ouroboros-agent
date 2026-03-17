import { useMemo } from "react"
import { Brain, RefreshCw, Tag } from "lucide-react"
import { useSessionFacts } from "../hooks/use-session-facts"
import { timeAgo } from "@/lib/utils"
import { cn } from "@/lib/utils"
import type { SessionFact } from "@/api/types"

interface Props {
  sessionId: string | null
  enabled: boolean
}

const CATEGORY_COLORS: Record<string, string> = {
  general: "bg-slate-100 text-slate-600",
  preference: "bg-blue-100 text-blue-600",
  context: "bg-purple-100 text-purple-600",
  task: "bg-green-100 text-green-600",
}

function categoryColor(category: string): string {
  return CATEGORY_COLORS[category] ?? "bg-slate-100 text-slate-600"
}

export function SessionMemoryPanel({ sessionId, enabled }: Props) {
  const { data: facts = [], isLoading, isFetching, refetch } = useSessionFacts(sessionId, enabled)

  const grouped = useMemo(() => {
    if (facts.length === 0) return []
    const categories = new Map<string, SessionFact[]>()
    for (const f of facts) {
      const list = categories.get(f.category) ?? []
      list.push(f)
      categories.set(f.category, list)
    }
    return Array.from(categories.entries()).map(([category, items]) => ({ category, items }))
  }, [facts])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-48 text-sm text-slate-400">
        加载中...
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-5 py-2.5 border-b border-slate-100 shrink-0">
        <span className="text-xs text-slate-500">{facts.length} 条记忆</span>
        <button
          onClick={() => void refetch()}
          disabled={isFetching}
          className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600 disabled:opacity-40 transition-colors"
          title="刷新记忆"
        >
          <RefreshCw className={cn("h-3.5 w-3.5", isFetching && "animate-spin")} />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto">
        {facts.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-48 text-center">
            <Brain className="h-8 w-8 text-slate-300 mb-2" />
            <p className="text-sm text-slate-400">此会话暂无记忆</p>
          </div>
        ) : (
          <div className="p-4 space-y-4">
            {grouped.map(({ category, items }) => (
              <div key={category}>
                <div className="flex items-center gap-1.5 mb-2">
                  <Tag className="h-3 w-3 text-slate-400" />
                  <span className={cn("text-[11px] font-medium px-1.5 py-0.5 rounded", categoryColor(category))}>
                    {category}
                  </span>
                  <span className="text-[11px] text-slate-400">{items.length}</span>
                </div>
                <div className="space-y-1.5">
                  {items.map((fact) => (
                    <div key={fact.id} className="flex items-start gap-2 px-3 py-2 rounded-md bg-slate-50 border border-slate-100">
                      <p className="text-sm text-slate-700 flex-1 wrap-break-word">{fact.fact}</p>
                      <span className="text-[11px] text-slate-400 shrink-0 mt-0.5">
                        {fact.createdAt ? timeAgo(fact.createdAt) : ""}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
