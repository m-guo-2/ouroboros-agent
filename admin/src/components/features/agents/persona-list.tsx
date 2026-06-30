import { useState, useEffect, useCallback } from "react"
import { Copy, Loader2, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useSkills } from "@/hooks/use-skills"
import { useSandboxTemplates } from "@/hooks/use-sandbox-templates"
import {
  usePersonas,
  useCreatePersona,
  useUpdatePersona,
  useClonePersona,
  useDeletePersona,
} from "@/hooks/use-personas"
import { settingsApi } from "@/api/settings"
import type { AgentProfile, AvailableModel, Persona, SandboxTemplate, SubagentModelConfig } from "@/api/types"

const NEWAPI_PROVIDER = "openai"

const SUBAGENT_PROFILES = [
  { key: "developer", label: "Developer" },
  { key: "file_analysis", label: "File Analysis" },
  { key: "web_research", label: "Web Research" },
  { key: "data_report", label: "Data Report" },
] as const

function formatModelLabel(provider?: string | null, model?: string | null) {
  if (!provider && !model) return "使用默认"
  const m = model ?? ""
  if (m) return m
  return provider === NEWAPI_PROVIDER ? "NewAPI" : provider || "使用默认"
}

function formatSandboxTemplateLabel(id: string | null | undefined, templates: SandboxTemplate[] | undefined) {
  if (!id) return "继承 Agent 默认"
  return templates?.find((t) => t.id === id)?.display_name ?? id
}

type PersonaFormMode = "create" | "edit"

interface PersonaFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  agentId: string
  agent: AgentProfile
  mode: PersonaFormMode
  persona: Persona | null
}

function PersonaFormDialog({
  open,
  onOpenChange,
  agentId,
  agent,
  mode,
  persona,
}: PersonaFormDialogProps) {
  const { data: skills } = useSkills()
  const { data: sandboxTemplates } = useSandboxTemplates()
  const createMutation = useCreatePersona()
  const updateMutation = useUpdatePersona()

  const [displayName, setDisplayName] = useState("")
  const [overridePrompt, setOverridePrompt] = useState(false)
  const [prompt, setPrompt] = useState("")
  const [overrideModel, setOverrideModel] = useState(false)
  const [model, setModel] = useState("")
  const [overrideSandbox, setOverrideSandbox] = useState(false)
  const [sandboxTemplateId, setSandboxTemplateId] = useState("office-worker")
  const [overrideSkills, setOverrideSkills] = useState(false)
  const [selectedSkills, setSelectedSkills] = useState<string[]>([])
  const [overrideSubModels, setOverrideSubModels] = useState(false)
  const [subagentModels, setSubagentModels] = useState<Record<string, SubagentModelConfig>>({})
  const [overrideSubSkills, setOverrideSubSkills] = useState(false)
  const [subagentSkills, setSubagentSkills] = useState<Record<string, string[]>>({})

  const [availableModels, setAvailableModels] = useState<AvailableModel[]>([])
  const [modelsLoading, setModelsLoading] = useState(false)
  const [modelsError, setModelsError] = useState("")
  const [showModelList, setShowModelList] = useState(false)

  const resetFromProps = useCallback(() => {
    if (mode === "edit" && persona) {
      setDisplayName(persona.displayName)
      const hasPrompt = persona.systemPrompt != null && persona.systemPrompt !== ""
      setOverridePrompt(hasPrompt)
      setPrompt(persona.systemPrompt ?? "")
      const hasModel = Boolean(persona.provider || persona.model)
      setOverrideModel(hasModel)
      setModel(persona.model ?? "")
      const hasSandbox = Boolean(persona.sandboxTemplateId)
      setOverrideSandbox(hasSandbox)
      setSandboxTemplateId(persona.sandboxTemplateId ?? agent.sandboxTemplateId ?? "office-worker")
      setOverrideSkills(persona.skills != null)
      setSelectedSkills(persona.skills ?? [...(agent.skills ?? [])])
      const hasSubM = persona.subagentModels != null && Object.keys(persona.subagentModels).length > 0
      setOverrideSubModels(hasSubM)
      setSubagentModels({ ...(persona.subagentModels ?? agent.subagentModels ?? {}) })
      const hasSubS =
        persona.subagentSkills != null &&
        Object.values(persona.subagentSkills).some((a) => (a?.length ?? 0) > 0)
      setOverrideSubSkills(hasSubS)
      setSubagentSkills({ ...(persona.subagentSkills ?? agent.subagentSkills ?? {}) })
    } else {
      setDisplayName("")
      setOverridePrompt(false)
      setPrompt("")
      setOverrideModel(false)
      setModel("")
      setOverrideSandbox(false)
      setSandboxTemplateId(agent.sandboxTemplateId ?? "office-worker")
      setOverrideSkills(false)
      setSelectedSkills([])
      setOverrideSubModels(false)
      setSubagentModels({})
      setOverrideSubSkills(false)
      setSubagentSkills({})
    }
    setAvailableModels([])
    setModelsError("")
    setShowModelList(false)
  }, [mode, persona, agent])

  useEffect(() => {
    if (open) resetFromProps()
  }, [open, resetFromProps])

  const fetchAvailableModels = async () => {
    setModelsLoading(true)
    setModelsError("")
    try {
      const res = await settingsApi.getProviderModels(NEWAPI_PROVIDER)
      if (res.success && res.data) {
        setAvailableModels(res.data)
        setShowModelList(true)
      } else {
        setModelsError((res as { error?: string }).error || "获取模型列表失败")
      }
    } catch {
      setModelsError("网络请求失败，请检查 NewAPI API Key 是否已配置")
    } finally {
      setModelsLoading(false)
    }
  }

  const toggleSkill = (skillId: string) => {
    setSelectedSkills((prev) =>
      prev.includes(skillId) ? prev.filter((id) => id !== skillId) : [...prev, skillId]
    )
  }

  const toggleSubagentSkill = (profile: string, skillId: string) => {
    setSubagentSkills((prev) => {
      const current = prev[profile] ?? []
      const next = current.includes(skillId)
        ? current.filter((id) => id !== skillId)
        : [...current, skillId]
      return { ...prev, [profile]: next }
    })
  }

  const buildPayload = (): Partial<Persona> => {
    const filteredSubagentModels: Record<string, SubagentModelConfig> = {}
    if (overrideSubModels) {
      for (const [profile, cfg] of Object.entries(subagentModels)) {
        if (cfg?.model) {
          filteredSubagentModels[profile] = { provider: NEWAPI_PROVIDER, model: cfg.model }
        }
      }
    }
    const filteredSubagentSkills: Record<string, string[]> = {}
    if (overrideSubSkills) {
      for (const [profile, ids] of Object.entries(subagentSkills)) {
        if (ids.length > 0) {
          filteredSubagentSkills[profile] = ids
        }
      }
    }
    return {
      displayName: displayName.trim(),
      systemPrompt: overridePrompt ? (prompt || null) : null,
      provider: overrideModel && model ? NEWAPI_PROVIDER : null,
      model: overrideModel ? (model || null) : null,
      sandboxTemplateId: overrideSandbox ? sandboxTemplateId : null,
      skills: overrideSkills ? selectedSkills : null,
      subagentModels: overrideSubModels ? filteredSubagentModels : null,
      subagentSkills: overrideSubSkills ? filteredSubagentSkills : null,
    }
  }

  const handleSubmit = async () => {
    if (!displayName.trim()) {
      window.alert("请填写显示名称")
      return
    }
    const payload = buildPayload()
    try {
      if (mode === "create") {
        await createMutation.mutateAsync({ agentId, data: payload })
      } else if (persona) {
        await updateMutation.mutateAsync({ agentId, id: persona.id, data: payload })
      }
      onOpenChange(false)
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "保存失败")
    }
  }

  const pending = createMutation.isPending || updateMutation.isPending

  const defaultPromptPreview = agent.systemPrompt ?? "（Agent 未配置系统提示词）"
  const defaultModelPreview = formatModelLabel(agent.provider, agent.model)
  const defaultSandboxPreview = formatSandboxTemplateLabel(agent.sandboxTemplateId ?? "office-worker", sandboxTemplates)
  const defaultSkillsCount = agent.skills?.length ?? 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{mode === "create" ? "新建 Persona" : "编辑 Persona"}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">
              显示名称 <span className="text-red-500">*</span>
            </label>
            <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
          </div>

          <div className="rounded-lg border border-slate-200 p-3 space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm font-medium text-slate-800">系统提示词</span>
              <Switch checked={overridePrompt} onCheckedChange={setOverridePrompt} />
            </div>
            {!overridePrompt ? (
              <p className="text-sm text-slate-400">使用默认配置</p>
            ) : (
              <Textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                rows={6}
                className="font-mono text-xs"
              />
            )}
            {!overridePrompt && (
              <p className="text-xs text-slate-500 line-clamp-3 whitespace-pre-wrap">
                预览：{defaultPromptPreview}
              </p>
            )}
          </div>

          <div className="rounded-lg border border-slate-200 p-3 space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm font-medium text-slate-800">主模型</span>
              <Switch checked={overrideModel} onCheckedChange={setOverrideModel} />
            </div>
            {!overrideModel ? (
              <p className="text-sm text-slate-400">使用默认配置</p>
            ) : (
              <div className="space-y-2">
                <div className="space-y-1.5">
                  <div className="flex gap-2">
                    <Input
                      value={model}
                      onChange={(e) => setModel(e.target.value)}
                      placeholder="查询 NewAPI 模型，或手动输入模型 ID"
                      className="flex-1"
                    />
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => fetchAvailableModels()}
                      disabled={modelsLoading}
                      className="shrink-0 whitespace-nowrap"
                    >
                      {modelsLoading ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <RefreshCw className="h-3.5 w-3.5" />
                      )}
                      查询 NewAPI 模型
                    </Button>
                  </div>
                  {modelsError && <p className="text-xs text-red-500">{modelsError}</p>}
                  {showModelList && availableModels.length > 0 && (
                    <div className="border border-slate-200 rounded-md bg-white max-h-48 overflow-y-auto shadow-sm">
                      {availableModels.map((m) => (
                        <button
                          key={m.id}
                          type="button"
                          className={`w-full text-left px-3 py-2 text-sm hover:bg-slate-50 border-b border-slate-100 last:border-0 ${
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
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            )}
            {!overrideModel && (
              <p className="text-xs text-slate-500">当前 Agent 默认：{defaultModelPreview}</p>
            )}
          </div>

          <div className="rounded-lg border border-slate-200 p-3 space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm font-medium text-slate-800">运行环境</span>
              <Switch checked={overrideSandbox} onCheckedChange={setOverrideSandbox} />
            </div>
            {!overrideSandbox ? (
              <p className="text-sm text-slate-400">使用默认配置</p>
            ) : (
              <select
                className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
                value={sandboxTemplateId}
                onChange={(e) => setSandboxTemplateId(e.target.value)}
              >
                {(sandboxTemplates ?? []).map((tpl) => (
                  <option key={tpl.id} value={tpl.id}>
                    {tpl.display_name}
                  </option>
                ))}
              </select>
            )}
            {!overrideSandbox && (
              <p className="text-xs text-slate-500">当前 Agent 默认：{defaultSandboxPreview}</p>
            )}
          </div>

          <div className="rounded-lg border border-slate-200 p-3 space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm font-medium text-slate-800">主 Agent 技能</span>
              <Switch checked={overrideSkills} onCheckedChange={setOverrideSkills} />
            </div>
            {!overrideSkills ? (
              <p className="text-sm text-slate-400">使用默认配置</p>
            ) : !skills || skills.length === 0 ? (
              <p className="text-sm text-slate-500">暂无可用技能</p>
            ) : (
              <div className="space-y-2 max-h-48 overflow-y-auto">
                {skills.map((skill) => {
                  const isBound = selectedSkills.includes(skill.id)
                  return (
                    <div
                      key={skill.id}
                      className={`rounded-md border p-2 transition-colors ${
                        isBound ? "border-brand-200 bg-brand-50/30" : "border-slate-200"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium text-slate-900">{skill.name}</p>
                          <p className="text-xs text-slate-500 truncate">{skill.description}</p>
                        </div>
                        <Switch checked={isBound} onCheckedChange={() => toggleSkill(skill.id)} />
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
            {!overrideSkills && (
              <p className="text-xs text-slate-500">
                当前 Agent 默认：{defaultSkillsCount > 0 ? `${defaultSkillsCount} 个技能` : "未绑定技能"}
              </p>
            )}
          </div>

          <div className="rounded-lg border border-slate-200 p-3 space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm font-medium text-slate-800">子 Agent 模型</span>
              <Switch checked={overrideSubModels} onCheckedChange={setOverrideSubModels} />
            </div>
            {!overrideSubModels ? (
              <p className="text-sm text-slate-400">使用默认配置</p>
            ) : (
              <div className="space-y-2">
                {SUBAGENT_PROFILES.map((p) => {
                  const cfg = subagentModels[p.key] ?? { provider: "", model: "" }
                  return (
                    <div
                      key={p.key}
                      className="flex flex-wrap items-center gap-2 p-2 border border-slate-200 rounded-md bg-slate-50/50"
                    >
                      <span className="text-xs font-medium text-slate-600 w-28 shrink-0">{p.label}</span>
                      <Input
                        value={cfg.model}
                        onChange={(e) => {
                          setSubagentModels((prev) => ({
                            ...prev,
                            [p.key]: { provider: NEWAPI_PROVIDER, model: e.target.value },
                          }))
                        }}
                        placeholder="留空继承"
                        className="flex-1 min-w-[120px] h-8 text-xs"
                      />
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          <div className="rounded-lg border border-slate-200 p-3 space-y-2">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm font-medium text-slate-800">子 Agent 技能</span>
              <Switch checked={overrideSubSkills} onCheckedChange={setOverrideSubSkills} />
            </div>
            {!overrideSubSkills ? (
              <p className="text-sm text-slate-400">使用默认配置</p>
            ) : !skills || skills.length === 0 ? (
              <p className="text-sm text-slate-500">暂无可用技能</p>
            ) : (
              <div className="space-y-3 max-h-56 overflow-y-auto">
                {SUBAGENT_PROFILES.map((p) => {
                  const profileSkills = subagentSkills[p.key] ?? []
                  return (
                    <div key={p.key} className="border border-slate-200 rounded-md p-2">
                      <p className="text-xs font-medium text-slate-700 mb-1.5">{p.label}</p>
                      <div className="flex flex-wrap gap-1.5">
                        {skills.map((skill) => {
                          const isSelected = profileSkills.includes(skill.id)
                          return (
                            <button
                              key={skill.id}
                              type="button"
                              className={`text-xs px-2 py-0.5 rounded-full border transition-colors ${
                                isSelected
                                  ? "bg-brand-600 text-white border-brand-600"
                                  : "bg-white text-slate-600 border-slate-200 hover:bg-slate-50"
                              }`}
                              onClick={() => toggleSubagentSkill(p.key, skill.id)}
                            >
                              {skill.name}
                            </button>
                          )
                        })}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="button" onClick={handleSubmit} disabled={pending}>
              {pending ? "保存中..." : "保存"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

export interface PersonaListProps {
  agentId: string
  agent: AgentProfile
}

export function PersonaList({ agentId, agent }: PersonaListProps) {
	const { data: personas, isLoading } = usePersonas(agentId)
	const { data: sandboxTemplates } = useSandboxTemplates()
	const cloneMutation = useClonePersona()
  const deleteMutation = useDeletePersona()

  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<PersonaFormMode>("create")
  const [editingPersona, setEditingPersona] = useState<Persona | null>(null)

  const [cloneOpen, setCloneOpen] = useState(false)
  const [cloneSource, setCloneSource] = useState<Persona | null>(null)
  const [cloneName, setCloneName] = useState("")

  const openCreate = () => {
    setFormMode("create")
    setEditingPersona(null)
    setFormOpen(true)
  }

  const openEdit = (p: Persona) => {
    setFormMode("edit")
    setEditingPersona(p)
    setFormOpen(true)
  }

  const openClone = (p: Persona) => {
    setCloneSource(p)
    setCloneName(`${p.displayName} 副本`)
    setCloneOpen(true)
  }

  const handleCloneConfirm = async () => {
    if (!cloneSource || !cloneName.trim()) {
      window.alert("请填写新名称")
      return
    }
    try {
      await cloneMutation.mutateAsync({
        agentId,
        id: cloneSource.id,
        displayName: cloneName.trim(),
      })
      setCloneOpen(false)
      setCloneSource(null)
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "克隆失败")
    }
  }

  const handleDelete = async (p: Persona) => {
    if (!window.confirm(`确认删除 Persona「${p.displayName}」？`)) return
    try {
      await deleteMutation.mutateAsync({ agentId, id: p.id })
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "删除失败")
    }
  }

  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-9 w-40" />
        <Skeleton className="h-32 rounded-lg" />
        <Skeleton className="h-32 rounded-lg" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-slate-700">Persona 列表</h3>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          新建 Persona
        </Button>
      </div>

      {!personas || personas.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-slate-500">
            暂无 Persona，点击「新建 Persona」开始配置。
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-1 lg:grid-cols-2">
          {personas.map((p) => {
            const skillsCount = p.skills?.length
            const skillsLabel =
              skillsCount == null ? "使用默认" : `${skillsCount} 个技能`
            const sandboxLabel = formatSandboxTemplateLabel(p.sandboxTemplateId, sandboxTemplates)
            return (
              <Card key={p.id}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base font-semibold">{p.displayName}</CardTitle>
                </CardHeader>
                <CardContent className="space-y-2 text-sm text-slate-600">
                  <p>模型：{formatModelLabel(p.provider, p.model)}</p>
                  <p>运行环境：{sandboxLabel}</p>
                  <p>技能：{skillsLabel}</p>
                  <p>已分配 {p.groupCount} 个群</p>
                  <div className="flex flex-wrap gap-2 pt-2">
                    <Button size="sm" variant="secondary" onClick={() => openEdit(p)}>
                      <Pencil className="h-3.5 w-3.5 mr-1" />
                      编辑
                    </Button>
                    <Button size="sm" variant="secondary" onClick={() => openClone(p)}>
                      <Copy className="h-3.5 w-3.5 mr-1" />
                      克隆
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-red-600 hover:text-red-700"
                      onClick={() => handleDelete(p)}
                      disabled={deleteMutation.isPending}
                    >
                      <Trash2 className="h-3.5 w-3.5 mr-1" />
                      删除
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}

      <PersonaFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        agentId={agentId}
        agent={agent}
        mode={formMode}
        persona={editingPersona}
      />

      <Dialog open={cloneOpen} onOpenChange={setCloneOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>克隆 Persona</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-slate-600 mb-2">为副本指定显示名称</p>
          <Input
            value={cloneName}
            onChange={(e) => setCloneName(e.target.value)}
            placeholder="新 Persona 名称"
          />
          <div className="flex justify-end gap-2 mt-4">
            <Button type="button" variant="secondary" onClick={() => setCloneOpen(false)}>
              取消
            </Button>
            <Button type="button" onClick={handleCloneConfirm} disabled={cloneMutation.isPending}>
              {cloneMutation.isPending ? "处理中..." : "确认克隆"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
