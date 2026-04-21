import { useState } from "react"
import {
  MessageSquare,
  Plus,
  Pencil,
  Trash2,
  RefreshCw,
  RotateCw,
  Loader2,
} from "lucide-react"
import { PageHeader } from "@/components/layout/page-header"
import { EmptyState } from "@/components/layout/empty-state"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import {
  useQiweiAccounts,
  useReloadQiweiRegistry,
  useRefreshQiweiProfile,
  useSoftDeleteQiweiAccount,
  useUpdateQiweiAccount,
} from "@/hooks/use-qiwei-accounts"
import type { QiweiAccount } from "@/api/qiwei-accounts"
import { AccountFormDialog } from "./account-form-dialog"
import { UnknownGuidPanel } from "./unknown-guid-panel"

function formatTs(ts: number) {
  if (!ts) return "-"
  const d = new Date(ts * 1000)
  return d.toLocaleString()
}

function AccountRow({ account }: { account: QiweiAccount }) {
  const [editing, setEditing] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const update = useUpdateQiweiAccount()
  const refresh = useRefreshQiweiProfile()
  const del = useSoftDeleteQiweiAccount()

  const handleToggle = (enabled: boolean) => {
    update.mutate({ id: account.id, data: { enabled } })
  }

  const handleRefresh = () => refresh.mutate(account.id)
  const handleDelete = () => {
    if (!confirmingDelete) {
      setConfirmingDelete(true)
      setTimeout(() => setConfirmingDelete(false), 3000)
      return
    }
    del.mutate(account.id)
  }

  return (
    <>
      <Card className="hover:shadow-sm transition-shadow">
        <CardContent className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-semibold text-slate-900 truncate">
                  {account.selfName || account.displayName || account.guid}
                </h3>
                <Badge variant={account.enabled ? "success" : "outline"}>
                  {account.enabled ? "启用" : "停用"}
                </Badge>
                {account.selfCorpName && (
                  <Badge variant="outline">{account.selfCorpName}</Badge>
                )}
              </div>

              <div className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs text-slate-500">
                <span className="text-slate-400">GUID</span>
                <span className="font-mono text-slate-600 truncate">{account.guid}</span>

                <span className="text-slate-400">ShortHash</span>
                <span className="font-mono text-slate-600">{account.shortHash}</span>

                <span className="text-slate-400">Token</span>
                <span className="font-mono text-slate-600">
                  {account.token_preview || "-"}
                </span>

                {account.agentId && (
                  <>
                    <span className="text-slate-400">Agent</span>
                    <span className="font-mono text-slate-600 truncate">
                      {account.agentId}
                    </span>
                  </>
                )}

                {account.selfUserId && (
                  <>
                    <span className="text-slate-400">企微 UserID</span>
                    <span className="font-mono text-slate-600 truncate">
                      {account.selfUserId}
                    </span>
                  </>
                )}

                <span className="text-slate-400">最近同步</span>
                <span className="text-slate-600">{formatTs(account.selfSyncedAt)}</span>

                {account.notes && (
                  <>
                    <span className="text-slate-400">备注</span>
                    <span className="text-slate-600">{account.notes}</span>
                  </>
                )}
              </div>
            </div>

            <div className="flex flex-col items-end gap-2">
              <Switch checked={account.enabled} onCheckedChange={handleToggle} />
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleRefresh}
                  disabled={refresh.isPending}
                  title="立即拉取企微 self profile"
                >
                  {refresh.isPending ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <RefreshCw className="h-3.5 w-3.5" />
                  )}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setEditing(true)}
                  title="编辑"
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleDelete}
                  disabled={del.isPending}
                  className={confirmingDelete ? "text-red-600" : ""}
                  title={confirmingDelete ? "再点一次确认删除" : "软删除"}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <AccountFormDialog
        open={editing}
        onOpenChange={setEditing}
        account={account}
      />
    </>
  )
}

export function QiweiAccountList() {
  const { data: accounts, isLoading, error } = useQiweiAccounts()
  const reload = useReloadQiweiRegistry()
  const [creating, setCreating] = useState(false)

  if (isLoading) {
    return (
      <div>
        <PageHeader title="企微账号" description="管理 channel-qiwei 的多账号配置" />
        <div className="mt-6 space-y-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-28 rounded-lg" />
          ))}
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div>
        <PageHeader title="企微账号" description="管理 channel-qiwei 的多账号配置" />
        <Card className="mt-6 border-red-200">
          <CardContent className="p-4 text-sm text-red-700">
            无法加载企微账号列表：{(error as Error).message}
            <p className="mt-2 text-xs text-red-600">
              请确认 channel-qiwei 进程已启动，且 /api/qiwei/_admin/* 可达。
            </p>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="企微账号"
        description="管理 channel-qiwei 的多账号配置"
        actions={
          <>
            <Button
              variant="secondary"
              onClick={() => reload.mutate()}
              disabled={reload.isPending}
              title="重新从 DB 加载 registry"
            >
              {reload.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RotateCw className="h-4 w-4" />
              )}
              重载 Registry
            </Button>
            <Button onClick={() => setCreating(true)}>
              <Plus className="h-4 w-4" />
              新建账号
            </Button>
          </>
        }
      />

      {!accounts || accounts.length === 0 ? (
        <EmptyState
          icon={MessageSquare}
          title="还没有企微账号"
          description="绑定一个企微第三方协议账号后，即可在此统一管理。"
          action={
            <Button onClick={() => setCreating(true)}>
              <Plus className="h-4 w-4" />
              新建账号
            </Button>
          }
          className="mt-12"
        />
      ) : (
        <div className="mt-6 space-y-3">
          {accounts.map((account) => (
            <AccountRow key={account.id} account={account} />
          ))}
        </div>
      )}

      <div className="mt-8 space-y-2">
        <h2 className="text-sm font-semibold text-slate-700">诊断</h2>
        <UnknownGuidPanel />
      </div>

      <AccountFormDialog open={creating} onOpenChange={setCreating} />
    </div>
  )
}
