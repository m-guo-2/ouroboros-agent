import { useState } from "react"
import { Cpu, Eye, EyeOff, Check, X, ExternalLink, RefreshCw, Loader2 } from "lucide-react"
import { PageHeader } from "@/components/layout/page-header"
import { EmptyState } from "@/components/layout/empty-state"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { useModels, useUpdateModel } from "@/hooks/use-models"
import { modelsApi } from "@/api/models"
import type { AvailableModel } from "@/api/types"

function ModelCard({ model }: { model: import("@/api/types").Model }) {
  const [editing, setEditing] = useState(false)
  const [apiKey, setApiKey] = useState("")
  const [showKey, setShowKey] = useState(false)
  const [baseUrl, setBaseUrl] = useState(model.baseUrl ?? "")
  const [modelId, setModelId] = useState(model.model)
  const updateMutation = useUpdateModel()

  // 模型发现
  const [availableModels, setAvailableModels] = useState<AvailableModel[]>([])
  const [modelListOpen, setModelListOpen] = useState(false)
  const [fetchingModels, setFetchingModels] = useState(false)
  const [fetchError, setFetchError] = useState("")

  const handleSave = async () => {
    await updateMutation.mutateAsync({
      id: model.id,
      data: {
        ...(apiKey ? { apiKey } : {}),
        baseUrl: baseUrl || undefined,
        model: modelId,
      },
    })
    setApiKey("")
    setEditing(false)
  }

  const handleToggle = async (enabled: boolean) => {
    await updateMutation.mutateAsync({ id: model.id, data: { enabled } })
  }

  const handleFetchModels = async () => {
    setFetchingModels(true)
    setFetchError("")
    try {
      const res = await modelsApi.getAvailableModels(model.id)
      if (res.success && res.data) {
        setAvailableModels(res.data)
        setModelListOpen(true)
      } else {
        setFetchError(res.error || "获取模型列表失败")
      }
    } catch {
      setFetchError("请求失败")
    } finally {
      setFetchingModels(false)
    }
  }

  const providerColors: Record<string, string> = {
    claude: "bg-amber-50 text-amber-700",
    openai: "bg-green-50 text-green-700",
    kimi: "bg-purple-50 text-purple-700",
    glm: "bg-blue-50 text-blue-700",
  }

  return (
    <Card className="hover:shadow-sm transition-shadow">
      <CardContent className="p-4">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-slate-100">
              <Cpu className="h-5 w-5 text-slate-600" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-slate-900">{model.name}</h3>
              <p className="text-xs text-slate-400 font-mono">{model.model}</p>
            </div>
          </div>
          <Switch checked={model.enabled} onCheckedChange={handleToggle} />
        </div>

        <div className="mt-3 flex items-center gap-2">
          <Badge className={providerColors[model.provider] ?? ""}>{model.provider}</Badge>
          {model.hasApiKey ? (
            <Badge variant="success">已配置</Badge>
          ) : (
            <Badge variant="danger">未配置</Badge>
          )}
        </div>

        {editing ? (
          <div className="mt-3 space-y-2">
            <div className="relative">
              <Input
                type={showKey ? "text" : "password"}
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder="API Key"
                className="pr-10"
              />
              <button
                type="button"
                onClick={() => setShowKey(!showKey)}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 cursor-pointer"
              >
                {showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            <Input
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              placeholder="Base URL (可选)"
            />

            {/* 模型选择 */}
            <div>
              <div className="flex gap-2">
                <Input
                  value={modelId}
                  onChange={(e) => setModelId(e.target.value)}
                  placeholder="模型 ID"
                  className="flex-1 font-mono text-xs"
                />
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={handleFetchModels}
                  disabled={fetchingModels || !model.hasApiKey}
                  title={model.hasApiKey ? "从 API 查询可用模型" : "请先配置 API Key"}
                  className="shrink-0"
                >
                  {fetchingModels ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <RefreshCw className="h-3.5 w-3.5" />
                  )}
                </Button>
              </div>
              {fetchError && <p className="text-xs text-red-500 mt-1">{fetchError}</p>}
              {modelListOpen && availableModels.length > 0 && (
                <div className="mt-1 border border-slate-200 rounded-md bg-white max-h-48 overflow-y-auto shadow-sm">
                  {availableModels.map((m) => (
                    <button
                      key={m.id}
                      type="button"
                      className={`w-full text-left px-3 py-1.5 text-xs hover:bg-slate-50 cursor-pointer border-b border-slate-100 last:border-0 transition-colors ${modelId === m.id ? "bg-blue-50 text-blue-700 font-medium" : "text-slate-600"
                        }`}
                      onClick={() => {
                        setModelId(m.id)
                        setModelListOpen(false)
                      }}
                    >
                      <span className="font-medium">{m.name}</span>
                      {m.name !== m.id && (
                        <span className="text-slate-400 ml-1.5 font-mono">{m.id}</span>
                      )}
                    </button>
                  ))}
                </div>
              )}
            </div>

            <div className="flex justify-end gap-2">
              <Button variant="ghost" size="sm" onClick={() => { setEditing(false); setModelListOpen(false) }}>
                <X className="h-3.5 w-3.5" />
              </Button>
              <Button size="sm" onClick={handleSave} disabled={updateMutation.isPending}>
                <Check className="h-3.5 w-3.5" />
                保存
              </Button>
            </div>
          </div>
        ) : (
          <div className="mt-3 flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => setEditing(true)}>
              <ExternalLink className="h-3.5 w-3.5" />
              配置
            </Button>
            <span className="text-xs text-slate-400">
              max {model.maxTokens} tokens · temp {model.temperature}
            </span>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export function ModelList() {
  const { data: models, isLoading } = useModels()

  if (isLoading) {
    return (
      <div>
        <PageHeader title="Models" description="管理 LLM 模型配置" />
        <div className="mt-6 grid grid-cols-1 md:grid-cols-2 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="h-40 rounded-lg" />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div>
      <PageHeader title="Models" description="管理 LLM 模型配置" />

      {!models || models.length === 0 ? (
        <EmptyState icon={Cpu} title="暂无模型" className="mt-12" />
      ) : (
        <div className="mt-6 grid grid-cols-1 md:grid-cols-2 gap-4">
          {models.map((model) => (
            <ModelCard key={model.id} model={model} />
          ))}
        </div>
      )}
    </div>
  )
}
