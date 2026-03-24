import { GitBranch, ExternalLink, Loader2 } from "lucide-react"
import { cn, truncate } from "@/lib/utils"
import type { ExecutionStep } from "@/api/types"

const PROFILE_LABEL: Record<string, string> = {
  developer: "开发",
  file_analysis: "文件分析",
  web_research: "联网检索",
  data_report: "数据报告",
}

interface SubagentStepCardProps {
  callStep: ExecutionStep
  resultStep?: ExecutionStep
  onViewTrace?: (subTraceId: string, name: string) => void
}

export function SubagentStepCard({ callStep, resultStep, onViewTrace }: SubagentStepCardProps) {
  const input = callStep.toolInput as Record<string, unknown> | undefined
  const name = (input?.name as string) ?? "subagent"
  const profile = (input?.profile as string) ?? "developer"
  const task = (input?.task as string) ?? ""
  const profileLabel = PROFILE_LABEL[profile] ?? profile

  const subTraceId = resultStep?.subTraceId
  const hasResult = !!resultStep
  const isRunning = !hasResult

  return (
    <div className="group/row">
      <div className="flex items-start gap-2.5 py-2 px-2 rounded-md bg-indigo-50/50 border border-indigo-100">
        <div className="flex h-5 w-5 items-center justify-center rounded-full bg-indigo-100 shrink-0 mt-0.5">
          <GitBranch className="h-3 w-3 text-indigo-600" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5 mb-1">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-indigo-600">Subagent</span>
            <span className="text-[13px] font-medium text-slate-700">{name}</span>
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-indigo-100 text-indigo-700 shrink-0">
              {profileLabel}
            </span>
            {isRunning && (
              <span className="flex items-center gap-1 text-[10px] text-blue-600 ml-auto">
                <Loader2 className="h-3 w-3 animate-spin" />运行中
              </span>
            )}
          </div>

          {task && (
            <p className="text-[12px] text-slate-500 mb-1.5">{truncate(task, 120)}</p>
          )}

          {subTraceId && onViewTrace && (
            <button
              onClick={(e) => { e.stopPropagation(); onViewTrace(subTraceId, name) }}
              className={cn(
                "flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium transition-colors",
                "text-indigo-600 hover:bg-indigo-100"
              )}
            >
              <ExternalLink className="h-3 w-3" />查看执行过程
            </button>
          )}

          {!subTraceId && hasResult && (
            <span className="text-[10px] text-slate-400">无可用 trace</span>
          )}
        </div>
      </div>
    </div>
  )
}
