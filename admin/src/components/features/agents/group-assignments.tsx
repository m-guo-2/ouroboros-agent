import { useState } from "react"
import { Loader2, Plus, Trash2 } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  usePersonas,
  useGroupAssignments,
  useUnconfiguredGroups,
  useCreateGroupAssignment,
  useUpdateGroupAssignment,
  useDeleteGroupAssignment,
} from "@/hooks/use-personas"

function formatLastActive(ts: number) {
  const ms = ts > 1e12 ? ts : ts * 1000
  try {
    return new Date(ms).toLocaleString("zh-CN")
  } catch {
    return String(ts)
  }
}

export interface GroupAssignmentsProps {
  agentId: string
}

export function GroupAssignments({ agentId }: GroupAssignmentsProps) {
  const { data: personas } = usePersonas(agentId)
  const { data: assignments, isLoading: assignmentsLoading } = useGroupAssignments(agentId)
  const { data: unconfigured, isLoading: discoverLoading } = useUnconfiguredGroups(agentId)
  const createMutation = useCreateGroupAssignment()
  const updateMutation = useUpdateGroupAssignment()
  const deleteMutation = useDeleteGroupAssignment()

  const [pickPersonaByKey, setPickPersonaByKey] = useState<Record<string, string>>({})

  const [manualOpen, setManualOpen] = useState(false)
  const [manualSessionKey, setManualSessionKey] = useState("")
  const [manualGroupName, setManualGroupName] = useState("")
  const [manualPersonaId, setManualPersonaId] = useState("")

  const setPick = (sessionKey: string, personaId: string) => {
    setPickPersonaByKey((prev) => ({ ...prev, [sessionKey]: personaId }))
  }

  const handleConfirmUnconfigured = async (row: {
    sessionKey: string
    channelName: string
  }) => {
    const pid = pickPersonaByKey[row.sessionKey] ?? ""
    if (!pid) {
      window.alert("请先选择 Persona")
      return
    }
    try {
      await createMutation.mutateAsync({
        agentId,
        data: {
          sessionKey: row.sessionKey,
          groupName: row.channelName || row.sessionKey,
          personaId: pid,
        },
      })
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "创建失败")
    }
  }

  const handleIgnoreUnconfigured = async (row: {
    sessionKey: string
    channelName: string
  }) => {
    try {
      await createMutation.mutateAsync({
        agentId,
        data: {
          sessionKey: row.sessionKey,
          groupName: row.channelName || row.sessionKey,
        },
      })
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "操作失败")
    }
  }

  const handlePersonaChange = async (assignmentId: string, personaId: string) => {
    try {
      await updateMutation.mutateAsync({
        agentId,
        id: assignmentId,
        data: { personaId: personaId || undefined },
      })
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "更新失败")
    }
  }

  const handleDelete = async (assignmentId: string, groupName: string) => {
    if (!window.confirm(`确认移除群「${groupName}」的分配？`)) return
    try {
      await deleteMutation.mutateAsync({ agentId, id: assignmentId })
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "删除失败")
    }
  }

  const handleManualSubmit = async () => {
    const sk = manualSessionKey.trim()
    const gn = manualGroupName.trim()
    if (!sk || !gn) {
      window.alert("请填写会话键与群名称")
      return
    }
    try {
      await createMutation.mutateAsync({
        agentId,
        data: {
          sessionKey: sk,
          groupName: gn,
          personaId: manualPersonaId || undefined,
        },
      })
      setManualOpen(false)
      setManualSessionKey("")
      setManualGroupName("")
      setManualPersonaId("")
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "创建失败")
    }
  }

  const personaOptions = personas ?? []
  const busy = createMutation.isPending || updateMutation.isPending || deleteMutation.isPending

  const selectPersonaClass =
    "flex h-9 w-full max-w-[220px] rounded-md border border-slate-300 bg-white px-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"

  if (assignmentsLoading || discoverLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-24 rounded-lg" />
        <Skeleton className="h-40 rounded-lg" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">未分配的群</CardTitle>
          <p className="text-xs text-slate-500 font-normal">
            选择 Persona 并确认，或忽略（先占位，稍后再绑定 Persona）。
          </p>
        </CardHeader>
        <CardContent>
          {!unconfigured || unconfigured.length === 0 ? (
            <p className="text-sm text-slate-500">暂无待分配会话</p>
          ) : (
            <div className="space-y-3">
              {unconfigured.map((row) => (
                <div
                  key={row.sessionKey}
                  className="flex flex-col gap-3 rounded-lg border border-slate-200 p-3 sm:flex-row sm:items-end sm:justify-between"
                >
                  <div className="space-y-1 text-sm min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-slate-900">{row.channelName}</span>
                      <Badge variant="outline">{row.sourceChannel}</Badge>
                    </div>
                    <p className="text-xs text-slate-500 font-mono break-all">{row.sessionKey}</p>
                    <p className="text-xs text-slate-400">最近活跃：{formatLastActive(row.lastActive)}</p>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 shrink-0">
                    <select
                      className={selectPersonaClass}
                      value={pickPersonaByKey[row.sessionKey] ?? ""}
                      onChange={(e) => setPick(row.sessionKey, e.target.value)}
                    >
                      <option value="">选择 Persona…</option>
                      {personaOptions.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.displayName}
                        </option>
                      ))}
                    </select>
                    <Button
                      size="sm"
                      onClick={() => handleConfirmUnconfigured(row)}
                      disabled={busy}
                    >
                      确认
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => handleIgnoreUnconfigured(row)}
                      disabled={busy}
                    >
                      忽略
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">已分配的群</CardTitle>
        </CardHeader>
        <CardContent>
          {!assignments || assignments.length === 0 ? (
            <p className="text-sm text-slate-500">暂无已分配记录</p>
          ) : (
            <div className="space-y-2">
              {assignments.map((a) => (
                <div
                  key={a.id}
                  className="flex flex-col gap-2 rounded-lg border border-slate-200 p-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0 flex-1">
                    <p className="font-medium text-slate-900">{a.groupName}</p>
                    <p className="text-xs text-slate-500 font-mono break-all mt-0.5">{a.sessionKey}</p>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 shrink-0">
                    <select
                      className={selectPersonaClass}
                      value={a.personaId ?? ""}
                      onChange={(e) => handlePersonaChange(a.id, e.target.value)}
                      disabled={updateMutation.isPending}
                    >
                      <option value="">暂不指定 Persona</option>
                      {personaOptions.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.displayName}
                        </option>
                      ))}
                    </select>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-red-600 hover:text-red-700"
                      onClick={() => handleDelete(a.id, a.groupName)}
                      disabled={deleteMutation.isPending}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <div>
        <Button variant="secondary" size="sm" onClick={() => setManualOpen(true)}>
          <Plus className="h-4 w-4 mr-1" />
          手动添加
        </Button>
      </div>

      <Dialog open={manualOpen} onOpenChange={setManualOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>手动添加群分配</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div>
              <label className="text-sm font-medium text-slate-700 mb-1 block">会话键 sessionKey</label>
              <Input
                value={manualSessionKey}
                onChange={(e) => setManualSessionKey(e.target.value)}
                placeholder="例如 channel 侧会话标识"
                className="font-mono text-xs"
              />
            </div>
            <div>
              <label className="text-sm font-medium text-slate-700 mb-1 block">群名称</label>
              <Input
                value={manualGroupName}
                onChange={(e) => setManualGroupName(e.target.value)}
                placeholder="显示用名称"
              />
            </div>
            <div>
              <label className="text-sm font-medium text-slate-700 mb-1 block">Persona（可选）</label>
              <select
                className={selectPersonaClass + " max-w-none"}
                value={manualPersonaId}
                onChange={(e) => setManualPersonaId(e.target.value)}
              >
                <option value="">暂不指定</option>
                {personaOptions.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.displayName}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <div className="flex justify-end gap-2 mt-4">
            <Button type="button" variant="secondary" onClick={() => setManualOpen(false)}>
              取消
            </Button>
            <Button type="button" onClick={handleManualSubmit} disabled={createMutation.isPending}>
              {createMutation.isPending ? (
                <span className="inline-flex items-center">
                  <Loader2 className="h-4 w-4 animate-spin mr-1" />
                  提交中…
                </span>
              ) : (
                "保存"
              )}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
