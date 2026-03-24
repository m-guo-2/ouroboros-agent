import { GitBranch, RefreshCw } from "lucide-react"
import { useSessionSubagentJobs } from "../hooks/use-session-subagent-jobs"
import { timeAgo, cn } from "@/lib/utils"

interface Props {
  sessionId: string | null
  enabled: boolean
  onViewTrace?: (subTraceId: string) => void
}

const STATUS_BADGE: Record<string, { label: string; className: string }> = {
  queued: { label: "排队中", className: "bg-slate-100 text-slate-500" },
  running: { label: "运行中", className: "bg-blue-100 text-blue-700" },
  completed: { label: "已完成", className: "bg-green-100 text-green-700" },
  failed: { label: "失败", className: "bg-red-100 text-red-700" },
  canceled: { label: "已取消", className: "bg-amber-100 text-amber-700" },
}

const PROFILE_LABEL: Record<string, string> = {
  developer: "开发",
  file_analysis: "文件分析",
  web_research: "联网检索",
  data_report: "数据报告",
}

function formatTime(ms: number): string {
  return new Date(ms).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  })
}

export function SubagentJobsPanel({ sessionId, enabled, onViewTrace }: Props) {
  const { data: jobs = [], isLoading, isFetching, refetch } = useSessionSubagentJobs(sessionId, enabled)

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
        <span className="text-xs text-slate-500">{jobs.length} 个子任务</span>
        <button
          onClick={() => void refetch()}
          disabled={isFetching}
          className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600 disabled:opacity-40 transition-colors"
          title="刷新 Subagent 列表"
        >
          <RefreshCw className={cn("h-3.5 w-3.5", isFetching && "animate-spin")} />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto">
        {jobs.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-48 text-center">
            <GitBranch className="h-8 w-8 text-slate-300 mb-2" />
            <p className="text-sm text-slate-400">此会话暂无 Subagent 任务</p>
          </div>
        ) : (
          <div className="p-4 space-y-2">
            {jobs.map((job) => {
              const badge = STATUS_BADGE[job.status] ?? STATUS_BADGE.queued
              const profileLabel = PROFILE_LABEL[job.profile] ?? job.profile
              const isRunning = job.status === "running"

              return (
                <div
                  key={job.id}
                  className={cn(
                    "px-3 py-2.5 rounded-md bg-slate-50 border border-slate-100 transition-colors",
                    onViewTrace && job.subTraceId && "cursor-pointer hover:border-slate-300"
                  )}
                  onClick={() => onViewTrace?.(job.subTraceId)}
                >
                  <div className="flex items-start justify-between gap-2 mb-1">
                    <div className="flex items-center gap-1.5 min-w-0">
                      {isRunning && <span className="h-2 w-2 rounded-full bg-blue-500 animate-live-pulse shrink-0" />}
                      <span className="text-sm font-medium text-slate-700 truncate">{job.name}</span>
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-200 text-slate-600 shrink-0">
                        {profileLabel}
                      </span>
                    </div>
                    <span className={cn("text-[11px] font-medium px-1.5 py-0.5 rounded shrink-0", badge.className)}>
                      {badge.label}
                    </span>
                  </div>
                  <p className="text-xs text-slate-500 truncate mb-1.5">{job.task}</p>
                  <div className="flex items-center gap-3 text-[11px] text-slate-400">
                    <span>{formatTime(job.createdAt)}</span>
                    {job.impactCount > 0 && <span>{job.impactCount} 次工具调用</span>}
                    {job.updatedAt > job.createdAt && <span>{timeAgo(job.updatedAt)}</span>}
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
