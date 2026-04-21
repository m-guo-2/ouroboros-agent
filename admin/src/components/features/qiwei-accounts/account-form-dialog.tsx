import { useEffect, useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import {
  useCreateQiweiAccount,
  useUpdateQiweiAccount,
} from "@/hooks/use-qiwei-accounts"
import type { QiweiAccount } from "@/api/qiwei-accounts"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  // When `account` is provided, the dialog edits it; otherwise it creates a new one.
  account?: QiweiAccount
}

export function AccountFormDialog({ open, onOpenChange, account }: Props) {
  const editing = !!account
  const [guid, setGuid] = useState("")
  const [token, setToken] = useState("")
  const [displayName, setDisplayName] = useState("")
  const [agentId, setAgentId] = useState("")
  const [notes, setNotes] = useState("")
  const [enabled, setEnabled] = useState(true)
  const [error, setError] = useState("")

  const createMutation = useCreateQiweiAccount()
  const updateMutation = useUpdateQiweiAccount()
  const pending = createMutation.isPending || updateMutation.isPending

  // Sync form state to the target account whenever the dialog opens or the
  // target changes. We keep `token` empty in edit mode so the backend keeps
  // the existing value unless the operator explicitly types a new one.
  useEffect(() => {
    if (!open) return
    setError("")
    setToken("")
    if (account) {
      setGuid(account.guid)
      setDisplayName(account.displayName ?? "")
      setAgentId(account.agentId ?? "")
      setNotes(account.notes ?? "")
      setEnabled(account.enabled)
    } else {
      setGuid("")
      setDisplayName("")
      setAgentId("")
      setNotes("")
      setEnabled(true)
    }
  }, [open, account])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError("")
    try {
      if (editing && account) {
        await updateMutation.mutateAsync({
          id: account.id,
          data: {
            // Only send a token when the user typed a new one; empty string
            // would overwrite the stored value with an empty token.
            ...(token ? { token } : {}),
            displayName,
            agentId,
            notes,
            enabled,
          },
        })
      } else {
        if (!guid.trim() || !token.trim()) {
          setError("GUID 和 Token 都必填")
          return
        }
        await createMutation.mutateAsync({
          guid: guid.trim(),
          token: token.trim(),
          displayName: displayName.trim() || undefined,
          agentId: agentId.trim() || undefined,
          notes: notes.trim() || undefined,
          enabled,
        })
      }
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editing ? "编辑企微账号" : "新建企微账号"}</DialogTitle>
          <DialogDescription>
            {editing
              ? "修改账号配置。Token 留空则保持原值不变。"
              : "注册一个新的企微账号，保存后 registry 会自动热重载。"}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">
              GUID
            </label>
            <Input
              value={guid}
              onChange={(e) => setGuid(e.target.value)}
              placeholder="第三方协议的 guid"
              disabled={editing}
              autoFocus={!editing}
              className="font-mono text-xs"
            />
            {editing && (
              <p className="mt-1 text-xs text-slate-400">
                GUID 不可修改；如需换账号请新建再删旧条目。
              </p>
            )}
          </div>

          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">
              Token {editing && <span className="text-slate-400 font-normal">(可留空)</span>}
            </label>
            <Input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={
                editing
                  ? `当前: ${account?.token_preview ?? ""}`
                  : "调用第三方协议的 token"
              }
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm font-medium text-slate-700 mb-1.5 block">
                显示名 <span className="text-slate-400 font-normal">(可选)</span>
              </label>
              <Input
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="方便人看的别名"
              />
            </div>
            <div>
              <label className="text-sm font-medium text-slate-700 mb-1.5 block">
                Agent ID <span className="text-slate-400 font-normal">(可选)</span>
              </label>
              <Input
                value={agentId}
                onChange={(e) => setAgentId(e.target.value)}
                placeholder="绑定的 agent"
                className="font-mono text-xs"
              />
            </div>
          </div>

          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">
              备注
            </label>
            <Input
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="运营备注"
            />
          </div>

          <div className="flex items-center justify-between rounded-md border border-slate-200 px-3 py-2">
            <div>
              <p className="text-sm font-medium text-slate-700">启用</p>
              <p className="text-xs text-slate-400">关闭后 registry 会忽略这个账号</p>
            </div>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </div>

          {error && (
            <p className="rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
              {error}
            </p>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button
              variant="secondary"
              type="button"
              onClick={() => onOpenChange(false)}
              disabled={pending}
            >
              取消
            </Button>
            <Button type="submit" disabled={pending}>
              {pending ? "保存中..." : editing ? "保存" : "创建"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
