import { useCallback } from "react"
import { Activity, Search, Trash2, RefreshCw } from "lucide-react"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/shared/status-badge"
import { ChannelBadge } from "@/components/shared/channel-badge"
import { cn, timeAgo } from "@/lib/utils"
import type { AgentSessionListItem } from "@/api/types"

interface Props {
  sessions: AgentSessionListItem[] | undefined
  isLoading: boolean
  search: string
  onSearchChange: (q: string) => void
  selectedSessionId: string | null
  onSelectSession: (id: string) => void
  onDeleteSession: (e: React.MouseEvent, id: string) => void
  onRefresh: () => void
  isRefreshing: boolean
}

export function SessionList({
  sessions, isLoading, search, onSearchChange,
  selectedSessionId, onSelectSession, onDeleteSession,
  onRefresh, isRefreshing,
}: Props) {
  const filteredSessions = sessions
    ? (search
      ? sessions.filter((s) => {
        const q = search.toLowerCase()
        return (s.title?.toLowerCase().includes(q))
          || (s.channelName?.toLowerCase().includes(q))
          || (s.agentId?.toLowerCase().includes(q))
          || (s.sourceChannel?.toLowerCase().includes(q))
      })
      : sessions)
    : []

  return (
    <div className="w-60 flex-shrink-0 border-r border-slate-200 bg-white flex flex-col">
      <div className="p-3 border-b border-slate-100 flex-shrink-0">
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
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
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
                  onClick={() => onSelectSession(session.id)}
                  className={cn(
                    "w-full text-left px-3 py-2.5 transition-colors cursor-pointer group/item relative",
                    isSelected ? "bg-brand-50 border-r-2 border-brand-600" : "hover:bg-slate-50"
                  )}
                >
                  <div className="flex items-center gap-2">
                    {isProcessing && <span className="h-2 w-2 rounded-full bg-green-500 animate-live-pulse flex-shrink-0" />}
                    <span className={cn("text-xs font-medium truncate flex-1", isSelected ? "text-brand-700" : "text-slate-900")}>
                      {session.channelName || session.title || session.id?.slice(0, 10) || "未知会话"}
                    </span>
                    <button
                      onClick={(e) => onDeleteSession(e, session.id)}
                      className="opacity-0 group-hover/item:opacity-100 p-1 rounded hover:bg-red-100 text-slate-400 hover:text-red-600 transition-all flex-shrink-0"
                    >
                      <Trash2 className="h-3 w-3" />
                    </button>
                  </div>
                  <div className="flex items-center gap-1.5 mt-1">
                    {session.sourceChannel && <ChannelBadge channel={session.sourceChannel} />}
                    {session.executionStatus && <StatusBadge status={session.executionStatus} />}
                    <span className="text-[10px] text-slate-400 ml-auto">
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
          </div>
        )}
      </div>
    </div>
  )
}
