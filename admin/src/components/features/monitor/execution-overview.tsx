import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Activity, AlertTriangle, Bot, CheckCircle2, Clock3, Coins, MessageSquareReply, Search, Send, XCircle } from "lucide-react"
import { useNavigate } from "react-router-dom"
import { monitorApi, type ExecutionItem, type MonitorFilters } from "@/api/monitor"
import { channelLabel, cn, formatCost, formatDuration, timeAgo } from "@/lib/utils"

const ranges = [
  { label: "24 小时", ms: 24 * 60 * 60 * 1000 },
  { label: "7 天", ms: 7 * 24 * 60 * 60 * 1000 },
  { label: "30 天", ms: 30 * 24 * 60 * 60 * 1000 },
]

const outcomeMeta: Record<string, { label: string; className: string }> = {
  replied: { label: "已回复", className: "bg-emerald-50 text-emerald-700 border-emerald-200" },
  no_reply: { label: "无回复", className: "bg-amber-50 text-amber-700 border-amber-200" },
  send_failed: { label: "发送失败", className: "bg-rose-50 text-rose-700 border-rose-200" },
  execution_failed: { label: "执行失败", className: "bg-rose-50 text-rose-700 border-rose-200" },
  running: { label: "执行中", className: "bg-sky-50 text-sky-700 border-sky-200" },
}

export function ExecutionOverview() {
  const navigate = useNavigate()
  const [rangeMs, setRangeMs] = useState(ranges[0].ms)
  const [rangeStart, setRangeStart] = useState(() => Date.now() - ranges[0].ms)
  const [rangeEnd, setRangeEnd] = useState(() => Date.now() + 24 * 60 * 60 * 1000)
  const [search, setSearch] = useState("")
  const [status, setStatus] = useState("")
  const filters = useMemo<MonitorFilters>(() => ({
    from: rangeStart,
    to: rangeEnd,
    status: status || undefined,
    search: search.trim() || undefined,
    limit: 100,
  }), [rangeEnd, rangeStart, search, status])

  const overview = useQuery({
    queryKey: ["monitor-overview", filters],
    queryFn: async () => (await monitorApi.overview(filters)).data!,
    refetchInterval: 30_000,
  })
  const executions = useQuery({
    queryKey: ["monitor-executions", filters],
    queryFn: async () => (await monitorApi.executions(filters)).data ?? [],
    refetchInterval: query => query.state.data?.some(item => item.status === "running") ? 5_000 : 30_000,
  })

  const summary = overview.data?.summary
  const maxRequests = Math.max(1, ...(overview.data?.trend.map(point => point.requestCount) ?? [1]))
  const rows = executions.data ?? []
  const openExecution = (id: string) => {
    const path = `/monitor/executions/${encodeURIComponent(id)}`
    const opened = window.open(path, "_blank")
    if (opened) {
      opened.opener = null
      opened.focus()
    } else {
      navigate(path)
    }
  }

  return (
    <div className="h-full overflow-y-auto bg-[#f3f6fa]">
      <div className="mx-auto max-w-[1680px] space-y-5 px-5 py-5 lg:px-7">
        <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] text-sky-700">
              <Activity className="h-4 w-4" /> Execution Monitor
            </div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">执行可观测</h1>
            <p className="mt-1 text-sm text-slate-600">请求进入、执行状态、回复结果、失败位置、耗时与成本。</p>
          </div>
          <div className="flex rounded-lg border border-slate-200 bg-white p-1 shadow-sm" aria-label="统计时间范围">
            {ranges.map(range => (
              <button
                key={range.label}
                onClick={() => { const current = Date.now(); setRangeMs(range.ms); setRangeStart(current - range.ms); setRangeEnd(current + 24 * 60 * 60 * 1000) }}
                className={cn("cursor-pointer rounded-md px-3 py-1.5 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-sky-500",
                  rangeMs === range.ms ? "bg-slate-950 text-white" : "text-slate-600 hover:bg-slate-100")}
              >
                {range.label}
              </button>
            ))}
          </div>
        </header>

        {overview.error && <div role="alert" className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{overview.error.message}</div>}

        <section className="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-8" aria-label="核心指标">
          <Metric label="请求" value={summary?.requestCount ?? 0} icon={Bot} />
          <Metric label="回复率" value={summary ? `${(summary.replyRate * 100).toFixed(1)}%` : "—"} icon={MessageSquareReply} tone="green" />
          <Metric label="无回复" value={summary?.noReplyCount ?? 0} icon={AlertTriangle} tone="amber" />
          <Metric label="执行失败" value={summary?.executionFailedCount ?? 0} icon={XCircle} tone="red" />
          <Metric label="发送失败" value={summary?.sendFailedCount ?? 0} icon={Send} tone="red" />
          <Metric label="P95 耗时" value={summary ? formatDuration(summary.p95DurationMs) : "—"} icon={Clock3} />
          <Metric label="Token" value={summary ? compactNumber(summary.inputTokens + summary.outputTokens) : "—"} icon={Activity} />
          <Metric label={summary ? `已知成本 · ${(summary.costCoverage * 100).toFixed(0)}% 覆盖` : "已知成本"} value={summary ? formatCost(summary.totalCostUsd) : "—"} icon={Coins} />
        </section>

        <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
          <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-sm font-semibold text-slate-900">请求与回复趋势</h2>
                <p className="mt-0.5 text-xs text-slate-500">柱高表示请求量，绿色表示已回复。</p>
              </div>
              {summary && <span className="text-xs text-slate-500">平均耗时 {formatDuration(summary.avgDurationMs)}</span>}
            </div>
            <div className="mt-5 flex h-32 items-end gap-1 overflow-hidden" role="img" aria-label="请求与回复趋势图">
              {(overview.data?.trend ?? []).map(point => {
                const totalHeight = Math.max(4, point.requestCount / maxRequests * 100)
                const repliedHeight = point.requestCount ? point.repliedCount / point.requestCount * totalHeight : 0
                return (
                  <div key={point.timestamp} className="group relative flex min-w-2 flex-1 items-end" title={`${new Date(point.timestamp).toLocaleString()}：${point.requestCount} 请求，${point.repliedCount} 回复`}>
                    <div className="relative w-full rounded-t bg-slate-200" style={{ height: `${totalHeight}%` }}>
                      <div className="absolute inset-x-0 bottom-0 rounded-t bg-emerald-500" style={{ height: `${repliedHeight / totalHeight * 100}%` }} />
                    </div>
                  </div>
                )
              })}
              {!overview.isLoading && !overview.data?.trend.length && <div className="m-auto text-sm text-slate-500">当前时间范围暂无执行数据</div>}
            </div>
          </div>
          <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
            <h2 className="text-sm font-semibold text-slate-900">失败位置</h2>
            <p className="mt-0.5 text-xs text-slate-500">按最终失败阶段统计。</p>
            <div className="mt-4 space-y-3">
              {(overview.data?.failureBreakdown ?? []).map(item => (
                <div key={item.stage} className="flex items-center justify-between text-sm">
                  <span className="flex items-center gap-2 text-slate-700"><span className="h-2 w-2 rounded-full bg-rose-500" />{failureStageLabel(item.stage)}</span>
                  <span className="font-semibold tabular-nums text-slate-950">{item.count}</span>
                </div>
              ))}
              {!overview.isLoading && !overview.data?.failureBreakdown.length && <div className="flex items-center gap-2 text-sm text-emerald-700"><CheckCircle2 className="h-4 w-4" />暂无失败</div>}
            </div>
          </div>
        </section>

        <section className="grid gap-4">
          <div className="min-w-0 overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <div className="flex flex-col gap-3 border-b border-slate-200 px-4 py-3 md:flex-row md:items-center md:justify-between">
              <div>
                <h2 className="text-sm font-semibold text-slate-900">执行记录</h2>
                <p className="text-xs text-slate-500">{rows.length} 条，点击在新标签页查看完整执行过程。</p>
              </div>
              <div className="flex gap-2">
                <label className="relative block">
                  <span className="sr-only">搜索执行</span>
                  <Search className="pointer-events-none absolute left-2.5 top-2 h-3.5 w-3.5 text-slate-400" />
                  <input value={search} onChange={event => setSearch(event.target.value)} placeholder="请求、Trace、Session" className="h-8 w-52 rounded-md border border-slate-200 pl-8 pr-3 text-xs text-slate-900 outline-none focus:border-sky-500 focus:ring-2 focus:ring-sky-100" />
                </label>
                <select value={status} onChange={event => setStatus(event.target.value)} aria-label="执行状态" className="h-8 cursor-pointer rounded-md border border-slate-200 bg-white px-2 text-xs text-slate-700 outline-none focus:border-sky-500">
                  <option value="">全部状态</option>
                  <option value="running">执行中</option>
                  <option value="completed">已完成</option>
                  <option value="failed">执行失败</option>
                </select>
              </div>
            </div>
            {executions.error && <div role="alert" className="m-4 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-700">{executions.error.message}</div>}
            <div className="overflow-x-auto">
              <table className="w-full min-w-[900px] text-left text-xs">
                <thead className="bg-slate-50 text-slate-500">
                  <tr><th className="px-4 py-2.5 font-medium">请求</th><th className="px-3 py-2.5 font-medium">结果</th><th className="px-3 py-2.5 font-medium">Agent / 渠道</th><th className="px-3 py-2.5 font-medium">耗时</th><th className="px-3 py-2.5 font-medium">Token</th><th className="px-3 py-2.5 font-medium">成本</th><th className="px-4 py-2.5 font-medium">时间</th></tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {rows.map(item => <ExecutionRow key={item.id} item={item} onSelect={() => openExecution(item.id)} />)}
                </tbody>
              </table>
              {!executions.isLoading && rows.length === 0 && <div className="px-4 py-12 text-center text-sm text-slate-500">没有匹配的执行记录</div>}
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}

function Metric({ label, value, icon: Icon, tone = "blue" }: { label: string; value: string | number; icon: typeof Activity; tone?: "blue" | "green" | "amber" | "red" }) {
  const colors = { blue: "bg-sky-50 text-sky-700", green: "bg-emerald-50 text-emerald-700", amber: "bg-amber-50 text-amber-700", red: "bg-rose-50 text-rose-700" }
  return <div className="rounded-xl border border-slate-200 bg-white p-3 shadow-sm"><div className={cn("flex h-7 w-7 items-center justify-center rounded-lg", colors[tone])}><Icon className="h-3.5 w-3.5" /></div><p className="mt-3 text-[11px] font-medium text-slate-500">{label}</p><p className="mt-0.5 text-lg font-semibold tabular-nums text-slate-950">{value}</p></div>
}

function ExecutionRow({ item, onSelect }: { item: ExecutionItem; onSelect: () => void }) {
  const outcome = item.status === "running" ? outcomeMeta.running : outcomeMeta[item.outcome]
  return (
    <tr tabIndex={0} onClick={onSelect} onKeyDown={event => { if (event.key === "Enter" || event.key === " ") onSelect() }} className="cursor-pointer transition-colors hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-inset focus-visible:outline-sky-500">
      <td className="max-w-[360px] px-4 py-3"><p className="truncate text-sm font-medium text-slate-900">{item.requestSummary || "无文本请求"}</p><p className="mt-1 truncate font-mono text-[10px] text-slate-400">{item.id}</p></td>
      <td className="px-3 py-3"><span className={cn("inline-flex rounded-full border px-2 py-0.5 font-medium", outcome?.className ?? "border-slate-200 bg-slate-50 text-slate-600")}>{outcome?.label ?? item.status}</span>{item.failureStage && <p className="mt-1 text-[10px] text-rose-600">{failureStageLabel(item.failureStage)}</p>}</td>
      <td className="px-3 py-3 text-slate-700"><p>{item.agentId || "—"}</p><p className="mt-1 text-[10px] text-slate-400">{channelLabel(item.channel)}</p></td>
      <td className="px-3 py-3 tabular-nums text-slate-700">{item.status === "running" ? "执行中" : formatDuration(item.durationMs)}</td>
      <td className="px-3 py-3 tabular-nums text-slate-700">{compactNumber(item.inputTokens + item.outputTokens)}</td>
      <td className="px-3 py-3 tabular-nums text-slate-700">{item.costKnown ? formatCost(item.totalCostUsd) : "未配置"}</td>
      <td className="px-4 py-3 text-slate-500">{timeAgo(item.startedAt)}</td>
    </tr>
  )
}

function compactNumber(value: number) {
  return new Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 1 }).format(value)
}

function failureStageLabel(stage: string) {
  return ({ queue: "排队", config: "配置", model: "模型", tool: "工具", subagent: "Subagent", channel: "渠道发送", internal: "内部执行" } as Record<string, string>)[stage] ?? stage
}
