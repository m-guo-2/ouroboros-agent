import { useState, useMemo } from "react"
import { Wrench, CheckCircle2, XCircle, Clock, ChevronDown, ChevronRight, ExternalLink } from "lucide-react"
import { cn, formatDuration } from "@/lib/utils"
import type { ToolPair } from "../lib/types"

function safePretty(value: unknown): string {
  if (value == null) return ""
  if (typeof value === "string") {
    const text = value.trim()
    if (!text) return ""
    if ((text.startsWith("{") && text.endsWith("}")) || (text.startsWith("[") && text.endsWith("]"))) {
      try { return JSON.stringify(JSON.parse(text), null, 2) } catch { return value }
    }
    return value
  }
  try { return JSON.stringify(value, null, 2) } catch { return String(value) }
}

function escapeHtml(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
}

function openJsonInNewTab(title: string, value: unknown): void {
  const raw = safePretty(value)
  const html = `<!doctype html><html><head><meta charset="utf-8"/><title>${escapeHtml(title)}</title>
<style>body{margin:0;padding:16px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;background:#f8fafc;color:#0f172a}
pre{white-space:pre-wrap;word-break:break-word;background:#fff;border:1px solid #e2e8f0;border-radius:8px;padding:12px}</style>
</head><body><pre>${escapeHtml(raw)}</pre></body></html>`
  const blob = new Blob([html], { type: "text/html;charset=utf-8" })
  const url = URL.createObjectURL(blob)
  window.open(url, "_blank")
  setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

export function ToolCard({ pair }: { pair: ToolPair }) {
  const [expanded, setExpanded] = useState(false)
  const { call, result } = pair
  const success = result ? result.toolSuccess !== false : undefined
  const hasDetail = !!(call.toolInput || result?.toolResult || result?.error)

  const inputPretty = useMemo(() => safePretty(call.toolInput), [call.toolInput])
  const resultPretty = useMemo(() => safePretty(result?.error || result?.toolResult), [result])

  return (
    <div className="flex gap-2.5 group/tool">
      <div className="flex flex-col items-center pt-0.5">
        <div className={cn(
          "flex h-5 w-5 items-center justify-center rounded-full flex-shrink-0",
          success === false ? "bg-red-50" : success === true ? "bg-green-50" : "bg-brand-50"
        )}>
          {success === false
            ? <XCircle className="h-3 w-3 text-red-600" />
            : success === true
              ? <CheckCircle2 className="h-3 w-3 text-green-600" />
              : <Wrench className="h-3 w-3 text-brand-600" />}
        </div>
        <div className="w-px flex-1 bg-slate-200 mt-0.5" />
      </div>

      <div className="flex-1 pb-2.5 min-w-0">
        <div
          className={cn("flex items-center gap-1.5", hasDetail && "cursor-pointer")}
          onClick={() => hasDetail && setExpanded(!expanded)}
        >
          <span className="text-[10px] font-semibold uppercase tracking-wider text-brand-600">Tool</span>
          <span className="text-[13px] font-medium text-slate-700">{call.toolName}</span>
          {result?.toolDuration != null && (
            <span className="flex items-center gap-0.5 text-[10px] text-slate-400 ml-auto">
              <Clock className="h-2.5 w-2.5" />{formatDuration(result.toolDuration)}
            </span>
          )}
          {hasDetail && (
            <span className={cn("text-slate-300 transition-opacity opacity-0 group-hover/tool:opacity-100", !result?.toolDuration && "ml-auto")}>
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </span>
          )}
        </div>

        {expanded && (
          <div className="space-y-1.5 mt-1.5">
            {call.toolInput != null && (
              <div className="rounded-md border border-brand-100 bg-brand-50/40 overflow-hidden">
                <div className="px-2.5 py-0.5 bg-brand-100/50 text-[10px] font-semibold text-brand-700 uppercase tracking-wider flex items-center">
                  <span>Input</span>
                  <button className="ml-auto inline-flex items-center gap-1 text-[10px] font-medium text-brand-700 hover:text-brand-900"
                    onClick={(e) => { e.stopPropagation(); openJsonInNewTab(`Tool Input · ${call.toolName}`, call.toolInput) }}>
                    <ExternalLink className="h-3 w-3" />全文
                  </button>
                </div>
                <div className="p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-40 overflow-y-auto">
                  {inputPretty}
                </div>
              </div>
            )}
            {(result?.toolResult != null || result?.error) && (
              <div className={cn("rounded-md border overflow-hidden",
                result.toolSuccess === false ? "border-red-100 bg-red-50/40" : "border-green-100 bg-green-50/40")}>
                <div className={cn("px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider flex items-center",
                  result.toolSuccess === false ? "bg-red-100/50 text-red-700" : "bg-green-100/50 text-green-700")}>
                  <span>{result.toolSuccess === false ? "Error" : "Result"}</span>
                  {result.toolResult != null && !result.error && (
                    <button className={cn("ml-auto inline-flex items-center gap-1 text-[10px] font-medium",
                      result.toolSuccess === false ? "text-red-700 hover:text-red-900" : "text-green-700 hover:text-green-900")}
                      onClick={(e) => { e.stopPropagation(); openJsonInNewTab(`Tool Result · ${call.toolName}`, result.toolResult) }}>
                      <ExternalLink className="h-3 w-3" />全文
                    </button>
                  )}
                </div>
                <div className="p-2.5 text-[11px] font-mono text-slate-600 overflow-x-auto whitespace-pre-wrap max-h-40 overflow-y-auto">
                  {result.error
                    ? <span className="text-red-600">{result.error}</span>
                    : resultPretty}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
