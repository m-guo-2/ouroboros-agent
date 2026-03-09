import { useState } from "react"
import { FileText, X, Copy, Check } from "lucide-react"
import { Skeleton } from "@/components/ui/skeleton"
import { useLLMIO } from "../hooks/use-llm-io"

function stringify(v: unknown): string {
  return typeof v === "string" ? v : JSON.stringify(v, null, 2)
}

function CopyBtn({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = () => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <button onClick={handleCopy} className="ml-auto inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium hover:bg-white/60 transition-colors" title="复制">
      {copied ? <><Check className="h-3 w-3" />已复制</> : <><Copy className="h-3 w-3" />复制</>}
    </button>
  )
}

export function LLMIOViewer({ traceId, llmIORef, onClose }: { traceId: string; llmIORef: string; onClose: () => void }) {
  const { data, isLoading, error } = useLLMIO(traceId, llmIORef)

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div className="relative bg-white rounded-xl shadow-2xl w-[90vw] max-w-4xl max-h-[85vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-3 border-b border-slate-200 flex-shrink-0">
          <div className="flex items-center gap-2">
            <FileText className="h-4 w-4 text-brand-600" />
            <h3 className="text-sm font-semibold text-slate-900">LLM I/O</h3>
            <span className="text-xs font-mono text-slate-400">{llmIORef}</span>
          </div>
          <button onClick={onClose} className="p-1 rounded-md hover:bg-slate-100 text-slate-400 hover:text-slate-600">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-5">
          {isLoading && (
            <div className="space-y-3">
              <Skeleton className="h-6 w-32" />
              <Skeleton className="h-40 rounded-lg" />
              <Skeleton className="h-6 w-32" />
              <Skeleton className="h-40 rounded-lg" />
            </div>
          )}
          {error && <div className="text-sm text-red-600">加载失败: {String(error)}</div>}
          {data?.data && (
            <div className="space-y-4">
              {data.data.request != null && (
                <div className="rounded-lg border border-brand-100 overflow-hidden">
                  <div className="px-3 py-1.5 bg-brand-50 text-xs font-semibold text-brand-700 uppercase tracking-wider flex items-center">
                    Request
                    <CopyBtn text={stringify(data.data.request)} />
                  </div>
                  <pre className="p-3 text-[11px] font-mono text-slate-700 overflow-x-auto whitespace-pre-wrap max-h-[35vh] overflow-y-auto bg-white select-text">
                    {stringify(data.data.request)}
                  </pre>
                </div>
              )}
              {data.data.response != null && (
                <div className="rounded-lg border border-green-100 overflow-hidden">
                  <div className="px-3 py-1.5 bg-green-50 text-xs font-semibold text-green-700 uppercase tracking-wider flex items-center">
                    Response
                    <CopyBtn text={stringify(data.data.response)} />
                  </div>
                  <pre className="p-3 text-[11px] font-mono text-slate-700 overflow-x-auto whitespace-pre-wrap max-h-[35vh] overflow-y-auto bg-white select-text">
                    {stringify(data.data.response)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
