import { useState } from "react"
import { GitBranch, RefreshCw, ChevronDown, ExternalLink, AlertCircle, Wrench } from "lucide-react"
import { useSessionSubagentJobs } from "../hooks/use-session-subagent-jobs"
import { useSubagentJobDetail } from "../hooks/use-subagent-job-detail"
import { MarkdownContent } from "@/components/shared/markdown-content"
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

function JobCard({ jobId, name, profile, status, task, subTraceId, createdAt, updatedAt, impactCount, onViewTrace }: {
  jobId: string
  name: string
  profile: string
  status: string
  task: string
  subTraceId: string
  createdAt: number
  updatedAt: number
  impactCount: number
  onViewTrace?: (subTraceId: string) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const { data: detail, isLoading: isLoadingDetail } = useSubagentJobDetail(expanded ? jobId : null)

  const badge = STATUS_BADGE[status] ?? STATUS_BADGE.queued
  const profileLabel = PROFILE_LABEL[profile] ?? profile
  const isRunning = status === "running"

  return (
    <div className="rounded-md bg-slate-50 border border-slate-100 transition-colors">
      <button
        className="w-full text-left px-3 py-2.5 cursor-pointer hover:bg-slate-100/60 transition-colors"
        onClick={() => setExpanded(prev => !prev)}
      >
        <div className="flex items-start justify-between gap-2 mb-1">
          <div className="flex items-center gap-1.5 min-w-0">
            {isRunning && <span className="h-2 w-2 rounded-full bg-blue-500 animate-live-pulse shrink-0" />}
            <span className="text-sm font-medium text-slate-700 truncate">{name}</span>
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-200 text-slate-600 shrink-0">
              {profileLabel}
            </span>
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            <span className={cn("text-[11px] font-medium px-1.5 py-0.5 rounded", badge.className)}>
              {badge.label}
            </span>
            <ChevronDown className={cn("h-3.5 w-3.5 text-slate-400 transition-transform", expanded && "rotate-180")} />
          </div>
        </div>
        <p className={cn("text-xs text-slate-500 mb-1.5", !expanded && "line-clamp-2")}>{task}</p>
        <div className="flex items-center gap-3 text-[11px] text-slate-400">
          <span>{formatTime(createdAt)}</span>
          {impactCount > 0 && <span>{impactCount} 次工具调用</span>}
          {updatedAt > createdAt && <span>{timeAgo(updatedAt)}</span>}
        </div>
      </button>

      {expanded && (
        <div className="px-3 pb-3 pt-1 border-t border-slate-100 space-y-2.5">
          {isLoadingDetail ? (
            <div className="text-xs text-slate-400 py-2">加载详情...</div>
          ) : detail ? (
            <>
              {detail.status === "completed" && detail.result && (
                <div className="rounded-md bg-white border border-slate-200 p-3">
                  <div className="text-[11px] font-medium text-slate-500 mb-1.5">执行结果</div>
                  <MarkdownContent content={detail.result} className="text-sm text-slate-700 prose-sm max-w-none" />
                </div>
              )}

              {detail.status === "failed" && detail.error && (
                <div className="rounded-md bg-red-50 border border-red-200/60 p-3">
                  <div className="flex items-center gap-1.5 text-[11px] font-medium text-red-600 mb-1">
                    <AlertCircle className="h-3.5 w-3.5" />
                    错误
                  </div>
                  <p className="text-xs text-red-600 whitespace-pre-wrap">{detail.error}</p>
                </div>
              )}

              {detail.impacts && detail.impacts.length > 0 && (
                <div className="space-y-1">
                  <div className="text-[11px] font-medium text-slate-500">工具调用 ({detail.impacts.length})</div>
                  {detail.impacts.map((impact, idx) => (
                    <div key={idx} className="flex items-start gap-2 text-xs text-slate-600 py-1 px-2 rounded bg-white border border-slate-100">
                      <Wrench className="h-3 w-3 text-slate-400 mt-0.5 shrink-0" />
                      <div className="min-w-0">
                        <span className="font-medium text-slate-700">{impact.tool}</span>
                        {impact.summary && <span className="ml-1.5 text-slate-500">{impact.summary}</span>}
                        <div className="text-[10px] text-slate-400 mt-0.5">{formatTime(impact.timestamp)}</div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </>
          ) : null}

          {subTraceId && onViewTrace && (
            <button
              onClick={(e) => { e.stopPropagation(); onViewTrace(subTraceId) }}
              className="flex items-center gap-1.5 text-xs text-brand-600 hover:text-brand-800 font-medium mt-1"
            >
              <ExternalLink className="h-3 w-3" />
              查看执行过程
            </button>
          )}
        </div>
      )}
    </div>
  )
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
            {jobs.map((job) => (
              <JobCard
                key={job.id}
                jobId={job.id}
                name={job.name}
                profile={job.profile}
                status={job.status}
                task={job.task}
                subTraceId={job.subTraceId}
                createdAt={job.createdAt}
                updatedAt={job.updatedAt}
                impactCount={job.impactCount}
                onViewTrace={onViewTrace}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
