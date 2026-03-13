import { useMemo, useState } from "react"
import { Bot, Copy, Check } from "lucide-react"
import { copyToClipboard } from "@/lib/utils"
import { useLLMIO } from "../hooks/use-llm-io"

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <button onClick={handleCopy} className="p-0.5 rounded hover:bg-green-100 text-green-600/60 hover:text-green-700 transition-colors" title="复制">
      {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
    </button>
  )
}

function extractModelOutput(payload: Record<string, unknown> | undefined): string {
  if (!payload || payload.response == null) return ""
  const response = payload.response as Record<string, unknown>

  const content = response.content
  if (Array.isArray(content)) {
    const texts = content
      .map((item) => {
        if (!item || typeof item !== "object") return ""
        return typeof (item as Record<string, unknown>).text === "string"
          ? ((item as Record<string, unknown>).text as string) : ""
      })
      .filter(Boolean)
    if (texts.length > 0) return texts.join("\n")
  }

  const choices = response.choices
  if (Array.isArray(choices) && choices.length > 0) {
    const msg = (choices[0] as Record<string, unknown>).message as Record<string, unknown> | undefined
    if (typeof msg?.content === "string" && (msg.content as string).trim()) return msg.content as string
  }
  return ""
}

export function ModelOutputView({ traceId, llmIORef }: { traceId: string; llmIORef: string }) {
  const { data, isLoading } = useLLMIO(traceId, llmIORef)
  const payload = data?.data as Record<string, unknown> | undefined
  const output = useMemo(() => extractModelOutput(payload), [payload])

  if (isLoading) {
    return (
      <div className="flex gap-2.5">
        <div className="flex h-5 w-5 items-center justify-center rounded-full bg-green-50 flex-shrink-0">
          <Bot className="h-3 w-3 text-green-600" />
        </div>
        <div className="flex-1 pb-2 min-w-0">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-green-600">Model Output</span>
          <p className="text-[11px] text-slate-400 mt-0.5 animate-pulse">加载中...</p>
        </div>
      </div>
    )
  }

  if (!output) return null

  return (
    <div className="flex gap-2.5">
      <div className="flex flex-col items-center pt-0.5">
        <div className="flex h-5 w-5 items-center justify-center rounded-full bg-green-50 flex-shrink-0">
          <Bot className="h-3 w-3 text-green-600" />
        </div>
        <div className="w-px flex-1 bg-slate-200 mt-0.5" />
      </div>
      <div className="flex-1 pb-2.5 min-w-0">
        <div className="flex items-center gap-1.5">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-green-600">Model Output</span>
          <CopyButton text={output} />
        </div>
        <div className="mt-1 rounded-md border border-green-100 bg-green-50/30 p-2.5 text-[13px] text-slate-700 whitespace-pre-wrap break-words max-h-56 overflow-y-auto select-text">
          {output}
        </div>
      </div>
    </div>
  )
}
