import { useState } from "react"
import { useParams, useNavigate, Link } from "react-router-dom"
import { ArrowLeft, Save, Trash2, RefreshCw, Loader2 } from "lucide-react"
import { PageHeader } from "@/components/layout/page-header"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { Skeleton } from "@/components/ui/skeleton"
import { useAgent, useUpdateAgent, useDeleteAgent } from "@/hooks/use-agents"
import { useSkills } from "@/hooks/use-skills"
import { settingsApi } from "@/api/settings"
import type { AvailableModel } from "@/api/types"

const PROVIDERS = [
  { value: "anthropic", label: "Anthropic (Claude)" },
  { value: "openai", label: "OpenAI (GPT)" },
  { value: "moonshot", label: "Moonshot (Kimi)" },
  { value: "zhipu", label: "智谱 (GLM)" },
]

export function AgentDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: agent, isLoading } = useAgent(id)
  const { data: skills } = useSkills()
  const updateMutation = useUpdateAgent()
  const deleteMutation = useDeleteAgent()

  const [name, setName] = useState("")
  const [prompt, setPrompt] = useState("")
  const [provider, setProvider] = useState("")
  const [model, setModel] = useState("")
  const [selectedSkills, setSelectedSkills] = useState<string[]>([])
  const [isActive, setIsActive] = useState(true)
  const [initialized, setInitialized] = useState(false)

  // 模型查询相关
  const [availableModels, setAvailableModels] = useState<AvailableModel[]>([])
  const [modelsLoading, setModelsLoading] = useState(false)
  const [modelsError, setModelsError] = useState("")
  const [showModelList, setShowModelList] = useState(false)

  // Initialize form from agent data
  if (agent && !initialized) {
    setName(agent.displayName)
    setPrompt(agent.systemPrompt ?? "")
    setProvider(agent.provider ?? "")
    setModel(agent.model ?? "")
    setSelectedSkills(agent.skills ?? [])
    setIsActive(agent.isActive !== false)
    setInitialized(true)
  }

  const fetchAvailableModels = async (providerValue?: string) => {
    const p = providerValue ?? provider
    if (!p) {
      setModelsError("请先选择 LLM 提供商")
      return
    }
    setModelsLoading(true)
    setModelsError("")
    try {
      const res = await settingsApi.getProviderModels(p)
      if (res.success && res.data) {
        setAvailableModels(res.data)
        setShowModelList(true)
      } else {
        setModelsError((res as any).error || "获取模型列表失败")
      }
    } catch {
      setModelsError("网络请求失败，请检查 API Key 是否已配置")
    } finally {
      setModelsLoading(false)
    }
  }

  if (isLoading) {
    return (
      <div>
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-96 mt-6 rounded-lg" />
      </div>
    )
  }

  if (!agent) {
    return <div className="text-sm text-slate-500">Agent 未找到</div>
  }

  const handleSave = async () => {
    await updateMutation.mutateAsync({
      id: agent.id,
      data: {
        displayName: name,
        systemPrompt: prompt,
        provider: provider || undefined,
        model: model || undefined,
        skills: selectedSkills,
        isActive,
      },
    })
  }

  const handleDelete = async () => {
    if (!confirm("确认删除此 Agent？")) return
    await deleteMutation.mutateAsync(agent.id)
    navigate("/agents")
  }

  const toggleSkill = (skillName: string) => {
    setSelectedSkills((prev) =>
      prev.includes(skillName) ? prev.filter((s) => s !== skillName) : [...prev, skillName]
    )
  }

  return (
    <div>
      <PageHeader
        title={agent.displayName}
        description={`ID: ${agent.id.slice(0, 12)}...`}
        actions={
          <div className="flex items-center gap-2">
            <Link to="/agents">
              <Button variant="ghost" size="sm">
                <ArrowLeft className="h-4 w-4" />
                返回
              </Button>
            </Link>
            {!agent.isDefault && (
              <Button variant="ghost" size="sm" onClick={handleDelete}>
                <Trash2 className="h-4 w-4 text-red-500" />
              </Button>
            )}
            <Button size="sm" onClick={handleSave} disabled={updateMutation.isPending}>
              <Save className="h-4 w-4" />
              {updateMutation.isPending ? "保存中..." : "保存"}
            </Button>
          </div>
        }
      />

      <Tabs defaultValue="config" className="mt-6">
        <TabsList>
          <TabsTrigger value="config">配置</TabsTrigger>
          <TabsTrigger value="skills">技能</TabsTrigger>
          <TabsTrigger value="channels">渠道</TabsTrigger>
        </TabsList>

        <TabsContent value="config">
          <Card>
            <CardContent className="p-6 space-y-4">
              <div className="flex items-center justify-between">
                <label className="text-sm font-medium">启用</label>
                <Switch checked={isActive} onCheckedChange={setIsActive} />
              </div>
              <div>
                <label className="text-sm font-medium text-slate-700 mb-1.5 block">名称</label>
                <Input value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div>
                <label className="text-sm font-medium text-slate-700 mb-1.5 block">LLM 提供商</label>
                <select
                  className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
                  value={provider}
                  onChange={(e) => {
                    setProvider(e.target.value)
                    setModel("")
                    setAvailableModels([])
                    setShowModelList(false)
                    setModelsError("")
                  }}
                >
                  <option value="">请选择提供商...</option>
                  {PROVIDERS.map((p) => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-sm font-medium text-slate-700 mb-1.5 block">模型</label>
                <div className="space-y-1.5">
                  <div className="flex gap-2">
                    <Input
                      value={model}
                      onChange={(e) => setModel(e.target.value)}
                      placeholder={provider ? "点击查询选择模型，或手动输入模型 ID" : "请先选择提供商"}
                      disabled={!provider}
                      className="flex-1"
                    />
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => fetchAvailableModels()}
                      disabled={modelsLoading || !provider}
                      className="shrink-0 whitespace-nowrap"
                    >
                      {modelsLoading ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <RefreshCw className="h-3.5 w-3.5" />
                      )}
                      {modelsLoading ? "查询中..." : "查询模型"}
                    </Button>
                  </div>
                  {modelsError && (
                    <p className="text-xs text-red-500">{modelsError}</p>
                  )}
                  {showModelList && availableModels.length > 0 && (
                    <div className="border border-slate-200 rounded-md bg-white max-h-60 overflow-y-auto shadow-sm">
                      {availableModels.map((m) => (
                        <button
                          key={m.id}
                          type="button"
                          className={`w-full text-left px-3 py-2 text-sm hover:bg-slate-50 cursor-pointer border-b border-slate-100 last:border-0 transition-colors ${
                            model === m.id ? "bg-blue-50 text-blue-700 font-medium" : "text-slate-700"
                          }`}
                          onClick={() => {
                            setModel(m.id)
                            setShowModelList(false)
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
                  {showModelList && availableModels.length === 0 && !modelsLoading && !modelsError && (
                    <p className="text-xs text-slate-400">未查询到可用模型</p>
                  )}
                </div>
              </div>
              <div>
                <label className="text-sm font-medium text-slate-700 mb-1.5 block">系统提示词</label>
                <Textarea
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  rows={8}
                  className="font-mono text-xs"
                />
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="skills">
          <Card>
            <CardHeader>
              <CardTitle>技能绑定</CardTitle>
            </CardHeader>
            <CardContent>
              {!skills || skills.length === 0 ? (
                <p className="text-sm text-slate-500">暂无可用技能</p>
              ) : (
                <div className="space-y-2">
                  {skills.map((skill) => (
                    <label
                      key={skill.name}
                      className="flex items-center justify-between rounded-md border border-slate-200 p-3 cursor-pointer hover:bg-slate-50 transition-colors"
                    >
                      <div>
                        <p className="text-sm font-medium text-slate-900">{skill.name}</p>
                        <p className="text-xs text-slate-500 mt-0.5">{skill.description}</p>
                      </div>
                      <Switch
                        checked={selectedSkills.includes(skill.name)}
                        onCheckedChange={() => toggleSkill(skill.name)}
                      />
                    </label>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="channels">
          <Card>
            <CardHeader>
              <CardTitle>渠道绑定</CardTitle>
            </CardHeader>
            <CardContent>
              {!agent.channels || agent.channels.length === 0 ? (
                <p className="text-sm text-slate-500">暂未绑定渠道</p>
              ) : (
                <div className="space-y-2">
                  {agent.channels.map((ch) => (
                    <div key={`${ch.type}-${ch.identifier}`} className="flex items-center gap-3 p-3 border border-slate-200 rounded-md">
                      <Badge variant="brand">{ch.type}</Badge>
                      <span className="text-sm text-slate-700 font-mono">{ch.identifier}</span>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
