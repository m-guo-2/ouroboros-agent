import { AlertTriangle } from "lucide-react"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { useUnknownQiweiGuids } from "@/hooks/use-qiwei-accounts"

function formatRelative(at: number) {
  if (!at) return "-"
  const diffSec = Math.max(0, Math.floor(Date.now() / 1000 - at))
  if (diffSec < 60) return `${diffSec}s ago`
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`
  return `${Math.floor(diffSec / 86400)}d ago`
}

export function UnknownGuidPanel() {
  const { data: events, isLoading } = useUnknownQiweiGuids()

  if (isLoading) {
    return <Skeleton className="h-24 rounded-lg" />
  }

  if (!events || events.length === 0) {
    return (
      <Card>
        <CardContent className="py-4 px-4 text-xs text-slate-400">
          最近没有收到未知 GUID 的回调。
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardContent className="p-0">
        <div className="flex items-center gap-2 border-b border-slate-100 px-4 py-2.5">
          <AlertTriangle className="h-4 w-4 text-amber-500" />
          <p className="text-sm font-medium text-slate-700">
            未注册 GUID 的回调（最近 {events.length} 条）
          </p>
          <span className="text-xs text-slate-400 ml-auto">
            收到后请检查是否漏配账号
          </span>
        </div>
        <div className="max-h-72 overflow-y-auto">
          <table className="w-full text-xs">
            <thead className="bg-slate-50 text-slate-500">
              <tr>
                <th className="px-4 py-2 text-left font-medium">GUID</th>
                <th className="px-4 py-2 text-left font-medium">Cmd</th>
                <th className="px-4 py-2 text-left font-medium">MsgType</th>
                <th className="px-4 py-2 text-left font-medium">MsgSvrID</th>
                <th className="px-4 py-2 text-left font-medium">时间</th>
              </tr>
            </thead>
            <tbody>
              {events.map((ev, i) => (
                <tr
                  key={`${ev.guid}-${ev.at}-${i}`}
                  className="border-t border-slate-100 hover:bg-slate-50"
                >
                  <td className="px-4 py-2 font-mono">{ev.guid}</td>
                  <td className="px-4 py-2">
                    <Badge variant="outline">{ev.cmd}</Badge>
                  </td>
                  <td className="px-4 py-2">{ev.msgType}</td>
                  <td className="px-4 py-2 font-mono text-slate-400">
                    {ev.msgSvrId || "-"}
                  </td>
                  <td className="px-4 py-2 text-slate-500">
                    {formatRelative(ev.at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>
  )
}
