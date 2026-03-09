import { Archive } from "lucide-react"
import { timeAgo } from "@/lib/utils"
import type { CompactionData } from "@/api/types"

export function CompactionEvent({ data, onClick }: { data: CompactionData; onClick?: () => void }) {
  return (
    <div
      className="flex items-center gap-2 mx-5 my-2 px-3 py-1.5 rounded-md bg-amber-50/80 border border-amber-200/60 text-[11px] text-amber-700 cursor-pointer hover:bg-amber-100/60 transition-colors"
      onClick={onClick}
    >
      <Archive className="h-3.5 w-3.5 flex-shrink-0" />
      <span className="font-medium">上下文压缩</span>
      <span className="text-amber-600/80">
        {data.archivedMessageCount} 条消息归档 · {data.tokenCountBefore.toLocaleString()} → {data.tokenCountAfter.toLocaleString()} tokens
      </span>
      <span className="ml-auto text-amber-500/70">{data.createdAt ? timeAgo(data.createdAt) : ""}</span>
    </div>
  )
}
