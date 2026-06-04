import { useMemo, useState } from "react"
import {
  Bot,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock,
  ExternalLink,
  FileText,
  MessageSquareText,
  Wrench,
  XCircle,
  Zap,
} from "lucide-react"
import { cn, formatDuration, formatCost, truncate } from "@/lib/utils"
import type { ExecutionStep } from "@/api/types"
import { LLMIOViewer } from "./llm-io-viewer"
import { safePretty, openJsonInNewTab } from "../lib/json-utils"

interface ToolPair {
  call: ExecutionStep
  result?: ExecutionStep
}

interface ModelBlock {
  iteration: number
  thinkings: ExecutionStep[]
  llmCall?: ExecutionStep
}

export function RoundDetail({ steps, traceId, isRunning, onViewSubagentTrace, defaultExpanded }: {
  steps: ExecutionStep[]
  traceId?: string
  isRunning?: boolean
  onViewSubagentTrace?: (subTraceId: string, name: string) => void
  defaultExpanded?: boolean
}) {
  const view = useMemo(() => buildExecutionView(steps), [steps])

  if (steps.length === 0) {
    return <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50 p-6 text-center text-sm text-slate-400">暂无执行事件</div>
  }

  return (
    <div className="space-y-4">
      <ExecutionSummary view={view} isRunning={isRunning} />

      <ExecutionSection
        number="1"
        title="模型判断"
        subtitle="模型如何理解问题、决定下一步"
        icon={Bot}
        tone="violet"
      >
        {view.modelBlocks.length > 0 ? (
          <div className="space-y-3">
            {view.modelBlocks.map((block) => (
              <ModelBlockCard
                key={block.iteration}
                block={block}
                traceId={traceId}
                defaultExpanded={defaultExpanded ?? block.iteration === view.modelBlocks[0]?.iteration}
              />
            ))}
          </div>
        ) : (
          <EmptyLine>没有记录到模型判断。</EmptyLine>
        )}
      </ExecutionSection>

      <ExecutionSection
        number="2"
        title="工具调用"
        subtitle="调用了哪些工具、输入是什么、返回了什么"
        icon={Wrench}
        tone="cyan"
      >
        {view.toolPairs.length > 0 ? (
          <div className="space-y-3">
            {view.toolPairs.map((pair, index) => (
              <ToolPairCard
                key={`${pair.call.toolCallId ?? pair.call.index}-${index}`}
                pair={pair}
                onViewSubagentTrace={onViewSubagentTrace}
                defaultExpanded={defaultExpanded ?? pair.result?.toolSuccess === false}
              />
            ))}
          </div>
        ) : (
          <EmptyLine>本轮没有工具调用。</EmptyLine>
        )}
      </ExecutionSection>

      <ExecutionSection
        number="3"
        title="输出与结果"
        subtitle="最终说了什么，或者失败在哪里"
        icon={MessageSquareText}
        tone={view.errors.length > 0 ? "red" : "green"}
      >
        <div className="space-y-3">
          {view.contentSteps.length > 0 ? (
            view.contentSteps.map((step) => (
              <ReadableBlock key={step.index} label="模型输出" value={step.content || "(空输出)"} />
            ))
          ) : view.errors.length === 0 ? (
            <EmptyLine>还没有记录最终输出。</EmptyLine>
          ) : null}

          {view.errors.map((step) => (
            <ReadableBlock key={step.index} label="错误" value={step.error || "(未知错误)"} tone="red" />
          ))}

          {isRunning && (
            <div className="flex items-center gap-2 rounded-lg border border-brand-200 bg-brand-50 px-3 py-2 text-sm text-brand-700">
              <span className="h-2 w-2 rounded-full bg-brand-500 animate-live-pulse" />
              执行仍在进行，结果会继续更新。
            </div>
          )}
        </div>
      </ExecutionSection>
    </div>
  )
}

function buildExecutionView(steps: ExecutionStep[]) {
  const modelMap = new Map<number, ModelBlock>()
  const toolCalls: ExecutionStep[] = []
  const toolResultsById = new Map<string, ExecutionStep>()
  const contentSteps: ExecutionStep[] = []
  const errors: ExecutionStep[] = []

  const getModelBlock = (iteration: number) => {
    if (!modelMap.has(iteration)) {
      modelMap.set(iteration, { iteration, thinkings: [] })
    }
    return modelMap.get(iteration)!
  }

  for (const step of steps) {
    const iteration = step.iteration ?? 0
    if (step.type === "thinking") {
      getModelBlock(iteration).thinkings.push(step)
    } else if (step.type === "llm_call") {
      getModelBlock(iteration).llmCall = step
    } else if (step.type === "tool_call") {
      toolCalls.push(step)
    } else if (step.type === "tool_result" && step.toolCallId) {
      toolResultsById.set(step.toolCallId, step)
    } else if (step.type === "content") {
      contentSteps.push(step)
    } else if (step.type === "error") {
      errors.push(step)
    }
  }

  const toolPairs = toolCalls.map((call) => ({
    call,
    result: call.toolCallId ? toolResultsById.get(call.toolCallId) : undefined,
  }))

  return {
    modelBlocks: Array.from(modelMap.values()).sort((a, b) => a.iteration - b.iteration),
    toolPairs,
    contentSteps,
    errors,
  }
}

function ExecutionSummary({ view, isRunning }: { view: ReturnType<typeof buildExecutionView>; isRunning?: boolean }) {
  const failedTools = view.toolPairs.filter((pair) => pair.result?.toolSuccess === false).length
  const status = view.errors.length > 0 || failedTools > 0
    ? "异常"
    : isRunning
      ? "处理中"
      : "完成"

  return (
    <div className="grid grid-cols-4 gap-2">
      <SummaryTile label="状态" value={status} tone={status === "异常" ? "red" : status === "处理中" ? "amber" : "green"} />
      <SummaryTile label="模型轮次" value={String(view.modelBlocks.length)} />
      <SummaryTile label="工具调用" value={String(view.toolPairs.length)} />
      <SummaryTile label="错误" value={String(view.errors.length + failedTools)} tone={view.errors.length + failedTools > 0 ? "red" : "muted"} />
    </div>
  )
}

function SummaryTile({ label, value, tone = "muted" }: { label: string; value: string; tone?: "muted" | "green" | "amber" | "red" }) {
  return (
    <div className={cn(
      "rounded-lg border px-3 py-2",
      tone === "green" && "border-emerald-200 bg-emerald-50",
      tone === "amber" && "border-amber-200 bg-amber-50",
      tone === "red" && "border-red-200 bg-red-50",
      tone === "muted" && "border-slate-200 bg-white"
    )}>
      <div className="text-[10px] font-medium text-slate-400">{label}</div>
      <div className={cn(
        "mt-1 text-sm font-semibold",
        tone === "green" && "text-emerald-700",
        tone === "amber" && "text-amber-700",
        tone === "red" && "text-red-700",
        tone === "muted" && "text-slate-800"
      )}>{value}</div>
    </div>
  )
}

function ExecutionSection({ number, title, subtitle, icon: Icon, tone, children }: {
  number: string
  title: string
  subtitle: string
  icon: typeof Bot
  tone: "violet" | "cyan" | "green" | "red"
  children: React.ReactNode
}) {
  return (
    <section className="overflow-hidden rounded-xl border border-slate-200 bg-white">
      <div className="flex items-center gap-3 border-b border-slate-100 bg-slate-50 px-4 py-3">
        <span className={cn(
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-xs font-semibold",
          tone === "violet" && "bg-violet-100 text-violet-700",
          tone === "cyan" && "bg-cyan-100 text-cyan-700",
          tone === "green" && "bg-emerald-100 text-emerald-700",
          tone === "red" && "bg-red-100 text-red-700"
        )}>{number}</span>
        <Icon className="h-4 w-4 text-slate-500" />
        <div className="min-w-0">
          <h4 className="text-sm font-semibold text-slate-900">{title}</h4>
          <p className="text-[11px] text-slate-500">{subtitle}</p>
        </div>
      </div>
      <div className="p-4">
        {children}
      </div>
    </section>
  )
}

function ModelBlockCard({ block, traceId, defaultExpanded }: { block: ModelBlock; traceId?: string; defaultExpanded?: boolean }) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? false)
  const [llmIOOpen, setLlmIOOpen] = useState(false)
  const thinkingText = block.thinkings
    .filter((step) => step.source !== "system")
    .map((step) => step.thinking ?? "")
    .join("\n")
    .trim()
  const systemText = block.thinkings
    .filter((step) => step.source === "system")
    .map((step) => step.thinking ?? "")
    .join("\n")
    .trim()
  const model = block.llmCall?.model ?? "unknown model"
  const hasLLMIO = !!block.llmCall?.llmIORef && !!traceId

  return (
    <div className="rounded-lg border border-violet-100 bg-violet-50/30">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full cursor-pointer items-start gap-3 px-3 py-3 text-left transition-colors hover:bg-violet-50"
      >
        <BrainLine />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold text-slate-900">第 {block.iteration || 1} 轮模型判断</span>
            <span className="rounded bg-white px-1.5 py-0.5 font-mono text-[10px] text-slate-500">{model}</span>
            {block.llmCall?.durationMs != null && <MetaChip icon={Clock}>{formatDuration(block.llmCall.durationMs)}</MetaChip>}
            {block.llmCall && <MetaChip icon={Zap}>{block.llmCall.inputTokens ?? "?"}↑ {block.llmCall.outputTokens ?? "?"}↓</MetaChip>}
            {block.llmCall?.costUsd != null && Number(block.llmCall.costUsd) > 0 && <span className="text-[10px] text-slate-400">{formatCost(block.llmCall.costUsd)}</span>}
          </div>
          <p className="mt-1 text-[12px] leading-relaxed text-slate-600">
            {thinkingText ? truncate(thinkingText, 180) : systemText ? truncate(systemText, 180) : "没有记录到可读思考内容。"}
          </p>
        </div>
        {expanded ? <ChevronDown className="mt-1 h-4 w-4 text-slate-400" /> : <ChevronRight className="mt-1 h-4 w-4 text-slate-400" />}
      </button>

      {expanded && (
        <div className="space-y-3 border-t border-violet-100 bg-white px-3 py-3">
          {thinkingText && <ReadableBlock label="模型思考" value={thinkingText} />}
          {systemText && <ReadableBlock label="系统上下文" value={systemText} tone="muted" />}
          {block.llmCall?.stopReason && <ReadableBlock label="停止原因" value={block.llmCall.stopReason} tone="muted" />}
          {hasLLMIO && (
            <button
              onClick={() => setLlmIOOpen(true)}
              className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-violet-200 bg-violet-50 px-2.5 py-1.5 text-[12px] font-medium text-violet-700 hover:bg-violet-100"
            >
              <FileText className="h-3.5 w-3.5" />
              查看完整模型输入/输出
            </button>
          )}
        </div>
      )}

      {llmIOOpen && hasLLMIO && (
        <LLMIOViewer traceId={traceId!} llmIORef={block.llmCall!.llmIORef!} onClose={() => setLlmIOOpen(false)} />
      )}
    </div>
  )
}

function ToolPairCard({ pair, onViewSubagentTrace, defaultExpanded }: {
  pair: ToolPair
  onViewSubagentTrace?: (subTraceId: string, name: string) => void
  defaultExpanded?: boolean
}) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? false)
  const success = pair.result?.toolSuccess !== false
  const pending = !pair.result
  const toolName = pair.call.toolName || "unknown_tool"
  const inputPreview = safePretty(pair.call.toolInput)
  const resultPreview = safePretty(pair.result?.error || pair.result?.toolResult)

  return (
    <div className={cn(
      "rounded-lg border",
      pending ? "border-amber-200 bg-amber-50/40" : success ? "border-cyan-100 bg-cyan-50/30" : "border-red-200 bg-red-50/40"
    )}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full cursor-pointer items-start gap-3 px-3 py-3 text-left transition-colors hover:bg-white/50"
      >
        <span className={cn(
          "mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg",
          pending ? "bg-amber-100 text-amber-700" : success ? "bg-cyan-100 text-cyan-700" : "bg-red-100 text-red-700"
        )}>
          {pending ? <Clock className="h-3.5 w-3.5" /> : success ? <CheckCircle2 className="h-3.5 w-3.5" /> : <XCircle className="h-3.5 w-3.5" />}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold text-slate-900">{toolName}</span>
            <span className={cn(
              "rounded px-1.5 py-0.5 text-[10px] font-medium",
              pending ? "bg-amber-100 text-amber-700" : success ? "bg-emerald-100 text-emerald-700" : "bg-red-100 text-red-700"
            )}>{pending ? "等待结果" : success ? "成功" : "失败"}</span>
            {pair.result?.toolDuration != null && <MetaChip icon={Clock}>{formatDuration(pair.result.toolDuration)}</MetaChip>}
          </div>
          <p className="mt-1 text-[12px] leading-relaxed text-slate-600">
            输入：{truncate(inputPreview || "无输入", 130)}
          </p>
          {pair.result && (
            <p className={cn("mt-0.5 text-[12px] leading-relaxed", success ? "text-slate-600" : "text-red-600")}>
              返回：{truncate(resultPreview || "无返回", 130)}
            </p>
          )}
        </div>
        {expanded ? <ChevronDown className="mt-1 h-4 w-4 text-slate-400" /> : <ChevronRight className="mt-1 h-4 w-4 text-slate-400" />}
      </button>

      {expanded && (
        <div className="space-y-3 border-t border-slate-100 bg-white px-3 py-3">
          <JsonBlock title="工具输入" value={pair.call.toolInput} name={`Tool Input · ${toolName}`} />
          {pair.result?.error ? (
            <ReadableBlock label="错误" value={pair.result.error} tone="red" />
          ) : (
            <JsonBlock title="工具返回" value={pair.result?.toolResult} name={`Tool Result · ${toolName}`} />
          )}
          {pair.result?.subTraceId && onViewSubagentTrace && (
            <button
              onClick={() => onViewSubagentTrace(pair.result!.subTraceId!, toolName)}
              className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-indigo-200 bg-indigo-50 px-2.5 py-1.5 text-[12px] font-medium text-indigo-700 hover:bg-indigo-100"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              查看 Subagent 执行过程
            </button>
          )}
        </div>
      )}
    </div>
  )
}

function JsonBlock({ title, value, name }: { title: string; value: unknown; name: string }) {
  if (value == null) return <ReadableBlock label={title} value="无" tone="muted" />
  const pretty = safePretty(value)
  return (
    <div className="overflow-hidden rounded-lg border border-slate-200">
      <div className="flex items-center gap-2 bg-slate-50 px-3 py-2">
        <span className="text-[11px] font-semibold text-slate-600">{title}</span>
        <button
          onClick={() => openJsonInNewTab(name, value)}
          className="ml-auto inline-flex cursor-pointer items-center gap-1 text-[11px] font-medium text-slate-500 hover:text-slate-900"
        >
          <ExternalLink className="h-3 w-3" />
          全文
        </button>
      </div>
      <pre className="max-h-56 overflow-auto p-3 text-[11px] leading-relaxed text-slate-600 whitespace-pre-wrap">{pretty}</pre>
    </div>
  )
}

function ReadableBlock({ label, value, tone = "default" }: { label: string; value: string; tone?: "default" | "muted" | "red" }) {
  return (
    <div className={cn(
      "rounded-lg border px-3 py-2",
      tone === "default" && "border-slate-200 bg-white",
      tone === "muted" && "border-slate-200 bg-slate-50",
      tone === "red" && "border-red-200 bg-red-50"
    )}>
      <div className={cn(
        "mb-1 text-[11px] font-semibold",
        tone === "red" ? "text-red-700" : "text-slate-500"
      )}>{label}</div>
      <div className={cn(
        "text-[13px] leading-relaxed whitespace-pre-wrap",
        tone === "red" ? "text-red-700" : "text-slate-700"
      )}>{value}</div>
    </div>
  )
}

function EmptyLine({ children }: { children: React.ReactNode }) {
  return <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-3 py-4 text-center text-sm text-slate-400">{children}</div>
}

function MetaChip({ icon: Icon, children }: { icon: typeof Clock; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1 rounded bg-white px-1.5 py-0.5 text-[10px] text-slate-500">
      <Icon className="h-3 w-3" />
      {children}
    </span>
  )
}

function BrainLine() {
  return (
    <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-violet-100 text-violet-700">
      <Bot className="h-3.5 w-3.5" />
    </span>
  )
}
