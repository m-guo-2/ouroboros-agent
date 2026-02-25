import { useState, useEffect } from "react"
import { Save, Eye, EyeOff, RefreshCw, ChevronDown, Loader2 } from "lucide-react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/page-header"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { settingsApi } from "@/api/settings"
import type { SettingGroup, SettingKeyDef, AvailableModel } from "@/api/types"
import { servicesApi } from "@/api/services"
import { StatusBadge } from "@/components/shared/status-badge"
import type { ServiceInfo } from "@/api/types"

function SecretInput({ value, onChange, placeholder }: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  const [show, setShow] = useState(false)
  return (
    <div className="relative">
      <Input
        type={show ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="pr-10"
      />
      <button
        type="button"
        onClick={() => setShow(!show)}
        className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 cursor-pointer"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

/** 模型选择器：支持从 provider API 自动发现模型 */
function ModelSelectInput({
  value,
  onChange,
  placeholder,
  provider,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  provider: string
}) {
  const [models, setModels] = useState<AvailableModel[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  const [open, setOpen] = useState(false)

  const fetchModels = async () => {
    if (!provider) {
      setError("请先选择 LLM 提供商")
      return
    }
    setLoading(true)
    setError("")
    try {
      const res = await settingsApi.getProviderModels(provider)
      if (res.success && res.data) {
        setModels(res.data)
        setOpen(true)
      } else {
        setError(res.error || "获取模型列表失败")
      }
    } catch {
      setError("网络请求失败")
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-1.5">
      <div className="flex gap-2">
        <Input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className="flex-1"
        />
        <Button
          type="button"
          variant="secondary"
          size="sm"
          onClick={fetchModels}
          disabled={loading || !provider}
          className="shrink-0 whitespace-nowrap"
        >
          {loading ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <RefreshCw className="h-3.5 w-3.5" />
          )}
          {loading ? "查询中..." : "查询模型"}
        </Button>
      </div>
      {error && (
        <p className="text-xs text-red-500">{error}</p>
      )}
      {open && models.length > 0 && (
        <div className="border border-slate-200 rounded-md bg-white max-h-60 overflow-y-auto shadow-sm">
          {models.map((m) => (
            <button
              key={m.id}
              type="button"
              className={`w-full text-left px-3 py-2 text-sm hover:bg-slate-50 cursor-pointer border-b border-slate-100 last:border-0 transition-colors ${
                value === m.id ? "bg-blue-50 text-blue-700 font-medium" : "text-slate-700"
              }`}
              onClick={() => {
                onChange(m.id)
                setOpen(false)
              }}
            >
              <span className="font-medium">{m.name}</span>
              {m.name !== m.id && (
                <span className="text-xs text-slate-400 ml-2 font-mono">{m.id}</span>
              )}
              {m.description && (
                <span className="text-xs text-slate-400 ml-2">— {m.description}</span>
              )}
            </button>
          ))}
        </div>
      )}
      {open && models.length === 0 && !loading && !error && (
        <p className="text-xs text-slate-400">未查询到可用模型</p>
      )}
    </div>
  )
}

export function SettingsPage() {
  const qc = useQueryClient()
  const [formValues, setFormValues] = useState<Record<string, string>>({})
  const [dirty, setDirty] = useState(false)

  const { data: settingsData, isLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: () => settingsApi.getAll(),
  })

  const { data: services } = useQuery({
    queryKey: ["services"],
    queryFn: async () => {
      const res = await servicesApi.getAll()
      return res.data ?? []
    },
    refetchInterval: 5000,
  })

  const saveMutation = useMutation({
    mutationFn: (values: Record<string, string>) => settingsApi.batchUpdate(values),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings"] })
      setDirty(false)
    },
  })

  // Service actions
  const serviceAction = useMutation({
    mutationFn: ({ name, action }: { name: string; action: "start" | "stop" | "restart" }) => {
      if (action === "start") return servicesApi.start(name)
      if (action === "stop") return servicesApi.stop(name)
      return servicesApi.restart(name)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["services"] }),
  })

  const settings = settingsData as unknown as { success: boolean; data: Record<string, string>; groups: Record<string, SettingGroup> } | undefined

  useEffect(() => {
    if (settings?.data) {
      setFormValues(settings.data)
    }
  }, [settings?.data])

  const updateValue = (key: string, value: string) => {
    setFormValues((prev) => ({ ...prev, [key]: value }))
    setDirty(true)
  }

  if (isLoading) {
    return (
      <div>
        <PageHeader title="Settings" description="系统配置" />
        <div className="mt-6 space-y-4">
          {[1, 2, 3].map((i) => <Skeleton key={i} className="h-40 rounded-lg" />)}
        </div>
      </div>
    )
  }

  const groups = settings?.groups ?? {}

  /** 根据字段类型渲染不同的输入控件 */
  const renderField = (item: SettingKeyDef) => {
    // provider 下拉选择
    if (item.type === "provider-select" && item.options) {
      return (
        <select
          className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          value={formValues[item.key] ?? ""}
          onChange={(e) => updateValue(item.key, e.target.value)}
        >
          <option value="">请选择提供商...</option>
          {item.options.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      )
    }

    // 模型选择（带查询按钮）
    if (item.type === "model-select") {
      const providerValue = item.providerKey ? (formValues[item.providerKey] ?? "") : ""
      return (
        <ModelSelectInput
          value={formValues[item.key] ?? ""}
          onChange={(v) => updateValue(item.key, v)}
          placeholder={item.placeholder}
          provider={providerValue}
        />
      )
    }

    // 密码输入
    if (item.secret) {
      return (
        <SecretInput
          value={formValues[item.key] ?? ""}
          onChange={(v) => updateValue(item.key, v)}
          placeholder={item.placeholder}
        />
      )
    }

    // 默认文本输入
    return (
      <Input
        value={formValues[item.key] ?? ""}
        onChange={(e) => updateValue(item.key, e.target.value)}
        placeholder={item.placeholder}
      />
    )
  }

  return (
    <div>
      <PageHeader
        title="Settings"
        description="系统配置管理"
        actions={
          dirty ? (
            <Button size="sm" onClick={() => saveMutation.mutate(formValues)} disabled={saveMutation.isPending}>
              <Save className="h-4 w-4" />
              {saveMutation.isPending ? "保存中..." : "保存更改"}
            </Button>
          ) : undefined
        }
      />

      <div className="mt-6 space-y-6">
        {/* Config groups */}
        {Object.entries(groups).map(([groupKey, group]) => (
          <Card key={groupKey}>
            <CardHeader>
              <CardTitle>{group.label}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {group.keys.map((item: SettingKeyDef) => (
                <div key={item.key}>
                  <label className="text-sm font-medium text-slate-700 mb-1.5 block">
                    {item.label}
                  </label>
                  {item.description && (
                    <p className="text-xs text-slate-400 mb-1">{item.description}</p>
                  )}
                  {renderField(item)}
                </div>
              ))}
            </CardContent>
          </Card>
        ))}

        {/* Services */}
        {services && services.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>服务管理</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                {services.map((svc: ServiceInfo) => (
                  <div key={svc.name} className="flex items-center justify-between p-3 border border-slate-200 rounded-md">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">{svc.label || svc.name}</span>
                        <StatusBadge status={svc.status} />
                      </div>
                      <p className="text-xs text-slate-400 mt-0.5">{svc.description}</p>
                    </div>
                    <div className="flex gap-1">
                      {svc.status === "stopped" ? (
                        <Button
                          variant="secondary" size="sm"
                          onClick={() => serviceAction.mutate({ name: svc.name, action: "start" })}
                          disabled={serviceAction.isPending}
                        >
                          启动
                        </Button>
                      ) : svc.status === "running" ? (
                        <>
                          <Button
                            variant="secondary" size="sm"
                            onClick={() => serviceAction.mutate({ name: svc.name, action: "restart" })}
                            disabled={serviceAction.isPending}
                          >
                            重启
                          </Button>
                          <Button
                            variant="ghost" size="sm"
                            onClick={() => serviceAction.mutate({ name: svc.name, action: "stop" })}
                            disabled={serviceAction.isPending}
                          >
                            停止
                          </Button>
                        </>
                      ) : null}
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  )
}
