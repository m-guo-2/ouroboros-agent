import { useState, useEffect, useRef } from "react"
import { Activity, Search, Trash2, RefreshCw } from "lucide-react"
import { Input } from "@/components/ui/input"
import { useTimeAgoTick } from "@/hooks/use-time-ago-tick"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/shared/status-badge"
import { ChannelBadge } from "@/components/shared/channel-badge"
import { cn, timeAgo, absoluteTime } from "@/lib/utils"
import type { AgentSessionListItem } from "@/api/types"

type StatusFilter = "" | "processing" | "idle" | "error"

const STATUS_CHIPS: { value: StatusFilter; label: string }[] = [
  { value: "", label: "全部" },
  { value: "processing", label: "处理中" },
  { value: "idle", label: "空闲" },
  { value: "error", label: "错误" },
]

interface Props {
  sessions: AgentSessionListItem[] | undefined
  isLoading: boolean
  selectedSessionId: string | null
  onSelectSession: (id: string) => void
  onDeleteSession: (e: React.MouseEvent, id: string) => void
  onRefresh: () => void
  isRefreshing: boolean
  hasMore?: boolean
  onLoadMore?: () => void
  isLoadingMore?: boolean
  onSearchChange?: (search: string) => void
  onStatusChange?: (status: string) => void
}

export function SessionList({
  sessions, isLoading,
  selectedSessionId, onSelectSession, onDeleteSession,
  onRefresh, isRefreshing, hasMore, onLoadMore, isLoadingMore,
  onSearchChange, onStatusChange,
}: Props) {
  const filteredSessions = sessions ?? []
  const timeAgoTick = useTimeAgoTick()

  const [searchInput, setSearchInput] = useState("")
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("")
  const debounceRef = useRef<ReturnType<typeof setTimeout>>()

  useEffect(() => {
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      onSearchChange?.(searchInput)
    }, 300)
    return () => clearTimeout(debounceRef.current)
  }, [searchInput, onSearchChange])

  const handleStatusChange = (status: StatusFilter) => {
    setStatusFilter(status)
    onStatusChange?.(status)
  }

  return (
    <div className="flex w-60 shrink-0 flex-col border-r border-slate-200 bg-white" data-time-ago-tick={timeAgoTick}>
      <div className="shrink-0 border-b border-slate-100 p-3">
        <div className="flex items-center justify-between mb-2">
          <h2 className="text-sm font-semibold text-slate-900">会话</h2>
          <div className="flex items-center gap-1.5">
            {sessions && <span className="text-[11px] text-slate-400">{sessions.length} 个</span>}
            <button
              onClick={onRefresh}
              disabled={isRefreshing}
              className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600 disabled:opacity-40 transition-colors"
              title="刷新会话列表"
            >
              <RefreshCw className={cn("h-3.5 w-3.5", isRefreshing && "animate-spin")} />
            </button>
          </div>
        </div>
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-400" />
          <Input
            placeholder="搜索会话..."
            className="h-8 pl-8 text-xs"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
        </div>
        <div className="flex items-center gap-1 mt-2">
          {STATUS_CHIPS.map((chip) => (
            <button
              key={chip.value}
              onClick={() => handleStatusChange(chip.value)}
              className={cn(
                "px-2 py-0.5 rounded-full text-[11px] font-medium transition-colors",
                statusFilter === chip.value
                  ? "bg-slate-900 text-white"
                  : "bg-slate-100 text-slate-500 hover:bg-slate-200"
              )}
            >
              {chip.label}
            </button>
          ))}
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
                  onClick={() => onSelectSession(session.id)}
                  className={cn(
                    "w-full text-left px-3 py-2.5 transition-colors cursor-pointer group/item relative",
                    isSelected ? "bg-brand-50 border-r-2 border-brand-600" : "hover:bg-slate-50",
                    session.executionStatus === "error" && !isSelected && "border-l-2 border-l-red-400"
                  )}
                >
                  <div className="flex items-center gap-2">
                    {isProcessing && <span className="h-2 w-2 shrink-0 animate-live-pulse rounded-full bg-green-500" />}
                    <span className={cn("flex-1 truncate text-xs font-medium", isSelected ? "text-brand-700" : "text-slate-900")}>
                      {session.channelName || session.title || session.id?.slice(0, 10) || "未知会话"}
                    </span>
                    <button
                      onClick={(e) => onDeleteSession(e, session.id)}
                      className="rounded p-1 text-slate-400 opacity-0 transition-all hover:bg-red-100 hover:text-red-600 group-hover/item:opacity-100 shrink-0"
                    >
                      <Trash2 className="h-3 w-3" />
                    </button>
                  </div>
                  <div className="flex items-center gap-1.5 mt-1">
                    {session.sourceChannel && <ChannelBadge channel={session.sourceChannel} />}
                    {session.executionStatus && <StatusBadge status={session.executionStatus} />}
                    <span className="text-[10px] text-slate-400 ml-auto" title={(session.updatedAt || session.createdAt) ? absoluteTime(session.updatedAt || session.createdAt) : ""}>
                      {(session.updatedAt || session.createdAt) ? timeAgo(session.updatedAt || session.createdAt) : ""}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 mt-0.5 text-[10px] text-slate-400">
                    <span>{session.agentId || "default"}</span>
                    {session.messageCount > 0 && <span>· {session.messageCount} 条</span>}
                  </div>
                </div>
              )
            })}
            {hasMore && (
              <button
                onClick={onLoadMore}
                disabled={isLoadingMore}
                className="w-full py-2.5 text-xs text-slate-400 hover:text-slate-600 hover:bg-slate-50 transition-colors disabled:opacity-40"
              >
                {isLoadingMore ? "加载中..." : "加载更多"}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
