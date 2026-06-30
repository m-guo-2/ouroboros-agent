import { useQuery } from "@tanstack/react-query"
import { ArrowLeft, RefreshCw } from "lucide-react"
import { useNavigate, useParams } from "react-router-dom"
import { monitorApi } from "@/api/monitor"
import { cn, formatCost, formatDuration, timeAgo } from "@/lib/utils"
import { DecisionInspector } from "./components/decision-inspector"

const outcomeLabel: Record<string, string> = {
  replied: "已回复",
  no_reply: "无回复",
  send_failed: "发送失败",
  execution_failed: "执行失败",
}

export function ExecutionDetailPage() {
  const navigate = useNavigate()
  const params = useParams()
  const id = params.id ?? ""
  const detail = useQuery({
    queryKey: ["monitor-execution", id],
    queryFn: async () => (await monitorApi.execution(id)).data!,
    enabled: !!id,
    refetchInterval: query => query.state.data?.execution.status === "running" ? 3_000 : false,
  })

  const data = detail.data
  const execution = data?.execution

  return (
    <div className="h-full overflow-y-auto bg-[#f3f6fa]">
      <div className="mx-auto flex min-h-full max-w-[1680px] flex-col gap-4 px-5 py-5 lg:px-7">
        <header className="rounded-xl border border-slate-200 bg-white px-4 py-4 shadow-sm">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="min-w-0">
              <button
                onClick={() => navigate("/monitor")}
                className="mb-3 inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                返回执行列表
              </button>
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-xl font-semibold tracking-tight text-slate-950">执行详情</h1>
                {execution && (
                  <span className={cn(
                    "rounded-full border px-2 py-0.5 text-xs font-medium",
                    execution.status === "running" && "border-sky-200 bg-sky-50 text-sky-700",
                    execution.status === "completed" && "border-emerald-200 bg-emerald-50 text-emerald-700",
                    execution.status === "failed" && "border-rose-200 bg-rose-50 text-rose-700"
                  )}>
                    {execution.status === "running" ? "执行中" : outcomeLabel[execution.outcome] ?? execution.status}
                  </span>
                )}
              </div>
              <p className="mt-1 truncate font-mono text-xs text-slate-500">{id}</p>
              {execution?.requestSummary && <p className="mt-2 max-w-4xl text-sm text-slate-700">{execution.requestSummary}</p>}
            </div>
            <button
              onClick={() => detail.refetch()}
              disabled={detail.isFetching}
              className="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border border-slate-200 bg-white px-3 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 disabled:cursor-default disabled:opacity-50"
            >
              <RefreshCw className={cn("h-3.5 w-3.5", detail.isFetching && "animate-spin")} />
              刷新
            </button>
          </div>

          {execution && (
            <div className="mt-4 grid grid-cols-2 gap-2 md:grid-cols-4 xl:grid-cols-8">
              <DetailMetric label="Agent" value={execution.agentId || "-"} />
              <DetailMetric label="模型" value={execution.model || "-"} />
              <DetailMetric label="耗时" value={execution.status === "running" ? "执行中" : formatDuration(execution.durationMs)} />
              <DetailMetric label="模型调用" value={String(execution.llmCallCount)} />
              <DetailMetric label="工具调用" value={String(execution.toolCallCount)} />
              <DetailMetric label="Token" value={String(execution.inputTokens + execution.outputTokens)} />
              <DetailMetric label="成本" value={execution.costKnown ? formatCost(execution.totalCostUsd) : "未配置"} />
              <DetailMetric label="开始" value={timeAgo(execution.startedAt)} />
            </div>
          )}
        </header>

        <section className="min-h-[720px] flex-1 overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
          {detail.error ? (
            <div role="alert" className="p-4 text-sm text-rose-700">{detail.error.message}</div>
          ) : data ? (
            <DecisionInspector
              trace={data.trace}
              lifecycleEvents={data.lifecycle}
              selectedMessageId={data.requests[0]?.id}
              isSessionProcessing={data.execution.status === "running"}
              onCollapse={() => navigate("/monitor")}
              onRefreshTrace={() => detail.refetch()}
              isRefreshingTrace={detail.isFetching}
            />
          ) : (
            <div className="p-4 text-sm text-slate-500">正在加载执行详情...</div>
          )}
        </section>
      </div>
    </div>
  )
}

function DetailMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
      <p className="text-[10px] font-medium text-slate-500">{label}</p>
      <p className="mt-0.5 truncate text-xs font-semibold text-slate-900">{value}</p>
    </div>
  )
}
