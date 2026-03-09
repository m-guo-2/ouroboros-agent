import { Brain, Cpu } from "lucide-react"
import { cn } from "@/lib/utils"
import type { ExecutionStep } from "@/api/types"

export function ThinkingView({ step }: { step: ExecutionStep }) {
  const isSystem = step.source === "system"
  const text = step.thinking ?? ""

  return (
    <div className="flex gap-2.5">
      <div className="flex flex-col items-center pt-0.5">
        <div className={cn("flex h-5 w-5 items-center justify-center rounded-full flex-shrink-0",
          isSystem ? "bg-purple-50" : "bg-slate-100")}>
          {isSystem
            ? <Cpu className="h-3 w-3 text-purple-500" />
            : <Brain className="h-3 w-3 text-slate-400" />
          }
        </div>
        <div className="w-px flex-1 bg-slate-200 mt-0.5" />
      </div>

      <div className="flex-1 pb-2 min-w-0">
        <span className={cn("text-[10px] font-semibold uppercase tracking-wider",
          isSystem ? "text-purple-500" : "text-slate-400")}>
          {isSystem ? "System" : "Think"}
        </span>
        <p className="text-[13px] text-slate-600 mt-0.5 leading-relaxed break-words whitespace-pre-wrap">
          {text}
        </p>
      </div>
    </div>
  )
}
