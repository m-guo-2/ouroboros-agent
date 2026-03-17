import { useState } from "react"
import { Clock, RefreshCw } from "lucide-react"
import { useSessionDelayedTasks } from "../hooks/use-session-delayed-tasks"
import { timeAgo, cn } from "@/lib/utils"

interface Props {
  sessionId: string | null
  enabled: boolean
}

const STATUS_BADGE: Record<string, { label: string; className: string }> = {
  pending: { label: "等待中", className: "bg-amber-100 text-amber-700" },
  dispatched: { label: "已执行", className: "bg-green-100 text-green-700" },
  cancelled: { label: "已取消", className: "bg-slate-100 text-slate-500" },
}

const FILTER_OPTIONS = [
  { value: "all", label: "全部" },
  { value: "pending", label: "等待中" },
  { value: "dispatched", label: "已执行" },
  { value: "cancelled", label: "已取消" },
] as const

function formatTime(ms: number): string {
  return new Date(ms).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export function SessionDelayedTasksPanel({ sessionId, enabled }: Props) {
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const { data: tasks = [], isLoading, isFetching, refetch } = useSessionDelayedTasks(sessionId, statusFilter, enabled)

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
        <div className="flex items-center gap-1">
          {FILTER_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              onClick={() => setStatusFilter(opt.value)}
              className={cn(
                "px-2 py-0.5 rounded text-[11px] font-medium transition-colors",
                statusFilter === opt.value
                  ? "bg-slate-900 text-white"
                  : "text-slate-500 hover:bg-slate-100"
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>
        <button
          onClick={() => void refetch()}
          disabled={isFetching}
          className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600 disabled:opacity-40 transition-colors"
          title="刷新定时任务"
        >
          <RefreshCw className={cn("h-3.5 w-3.5", isFetching && "animate-spin")} />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto">
        {tasks.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-48 text-center">
            <Clock className="h-8 w-8 text-slate-300 mb-2" />
            <p className="text-sm text-slate-400">此会话暂无定时任务</p>
          </div>
        ) : (
          <div className="p-4 space-y-2">
            {tasks.map((task) => {
              const badge = STATUS_BADGE[task.status] ?? STATUS_BADGE.pending
              return (
                <div key={task.id} className="px-3 py-2.5 rounded-md bg-slate-50 border border-slate-100">
                  <div className="flex items-start justify-between gap-2 mb-1.5">
                    <p className="text-sm text-slate-700 flex-1 wrap-break-word">{task.task}</p>
                    <span className={cn("text-[11px] font-medium px-1.5 py-0.5 rounded shrink-0", badge.className)}>
                      {badge.label}
                    </span>
                  </div>
                  <div className="flex items-center gap-3 text-[11px] text-slate-400">
                    <span>计划执行: {formatTime(task.executeAt)}</span>
                    <span>创建于: {task.createdAt ? timeAgo(task.createdAt) : ""}</span>
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
