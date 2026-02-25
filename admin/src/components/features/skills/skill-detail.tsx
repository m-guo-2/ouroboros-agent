import { useState, useEffect, useCallback } from "react"
import { useParams, Link, useNavigate } from "react-router-dom"
import { ArrowLeft, Trash2, Pencil, Save, X, Plus, Minus, Eye, Code } from "lucide-react"
import { PageHeader } from "@/components/layout/page-header"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { MarkdownContent } from "@/components/shared/markdown-content"
import { SkillVersions } from "./skill-versions"
import { useSkill, useToggleSkill, useDeleteSkill, useUpdateSkill } from "@/hooks/use-skills"
import type { SkillManifest } from "@/api/types"

type SkillTool = NonNullable<SkillManifest["tools"]>[number]

const typeOptions = [
  { value: "knowledge", label: "知识" },
  { value: "action", label: "动作" },
  { value: "hybrid", label: "混合" },
] as const

function emptyTool(): SkillTool {
  return {
    name: "",
    description: "",
    inputSchema: { type: "object", properties: {}, required: [] },
    executor: { type: "http", url: "", method: "POST" },
  }
}

export function SkillDetail() {
  const { name } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const { data: skill, isLoading } = useSkill(name)
  const toggleMutation = useToggleSkill()
  const deleteMutation = useDeleteSkill()
  const updateMutation = useUpdateSkill()

  const [editing, setEditing] = useState(false)
  const [description, setDescription] = useState("")
  const [type, setType] = useState<SkillManifest["type"]>("knowledge")
  const [triggers, setTriggers] = useState("")
  const [readme, setReadme] = useState("")
  const [tools, setTools] = useState<SkillTool[]>([])
  const [toolsJson, setToolsJson] = useState("")
  const [toolsJsonError, setToolsJsonError] = useState("")
  const [toolEditMode, setToolEditMode] = useState<"visual" | "json">("visual")
  const [changeSummary, setChangeSummary] = useState("")

  const syncFromSkill = useCallback(() => {
    if (!skill) return
    const m = skill.manifest
    setDescription(m.description)
    setType(m.type)
    setTriggers((m.triggers ?? []).join(", "))
    setReadme(skill.readme ?? "")
    setTools(m.tools ? structuredClone(m.tools) : [])
    setToolsJson(JSON.stringify(m.tools ?? [], null, 2))
    setToolsJsonError("")
    setChangeSummary("")
  }, [skill])

  useEffect(() => { syncFromSkill() }, [syncFromSkill])

  const enterEdit = () => {
    syncFromSkill()
    setEditing(true)
  }

  const cancelEdit = () => {
    syncFromSkill()
    setEditing(false)
  }

  const handleSave = async () => {
    if (!name) return

    let finalTools = tools
    if (toolEditMode === "json") {
      try {
        finalTools = JSON.parse(toolsJson)
        setToolsJsonError("")
      } catch {
        setToolsJsonError("JSON 格式错误")
        return
      }
    }

    const manifest: Partial<SkillManifest> = {
      description: description.trim(),
      type,
      triggers: triggers.split(/[,，\n]/).map(s => s.trim()).filter(Boolean),
      tools: finalTools,
    }

    await updateMutation.mutateAsync({
      name,
      manifest,
      readme: readme.trim(),
      changeSummary: changeSummary.trim() || undefined,
    })
    setEditing(false)
  }

  if (isLoading) {
    return (
      <div>
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-96 mt-6 rounded-lg" />
      </div>
    )
  }

  if (!skill) {
    return <div className="text-sm text-slate-500">技能未找到</div>
  }

  const m = skill.manifest

  return (
    <div>
      <PageHeader
        title={m.name}
        description={editing ? undefined : m.description}
        actions={
          <div className="flex items-center gap-2">
            {editing ? (
              <>
                <Button variant="secondary" size="sm" onClick={cancelEdit}>
                  <X className="h-4 w-4" /> 取消
                </Button>
                <Button size="sm" onClick={handleSave} disabled={updateMutation.isPending}>
                  <Save className="h-4 w-4" /> {updateMutation.isPending ? "保存中..." : "保存"}
                </Button>
              </>
            ) : (
              <>
                <Switch
                  checked={m.enabled}
                  onCheckedChange={(enabled) => toggleMutation.mutate({ name: m.name, enabled })}
                />
                <Button variant="secondary" size="sm" onClick={enterEdit}>
                  <Pencil className="h-4 w-4" /> 编辑
                </Button>
                <Button
                  variant="ghost" size="sm"
                  onClick={async () => {
                    if (!confirm("确认删除？")) return
                    await deleteMutation.mutateAsync(m.name)
                    navigate("/skills")
                  }}
                >
                  <Trash2 className="h-4 w-4 text-red-500" />
                </Button>
              </>
            )}
            <Link to="/skills">
              <Button variant="ghost" size="sm"><ArrowLeft className="h-4 w-4" /> 返回</Button>
            </Link>
          </div>
        }
      />

      <div className="mt-6 grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left: Info */}
        <Card className="lg:col-span-1">
          <CardHeader><CardTitle>信息</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            {editing ? (
              <>
                <div>
                  <label className="text-xs text-slate-400 mb-1 block">描述</label>
                  <Input value={description} onChange={(e) => setDescription(e.target.value)} />
                </div>
                <div>
                  <label className="text-xs text-slate-400 mb-1 block">类型</label>
                  <div className="flex gap-1.5">
                    {typeOptions.map(opt => (
                      <button
                        key={opt.value}
                        type="button"
                        onClick={() => setType(opt.value)}
                        className={`px-2.5 py-1 text-xs rounded-md border transition-colors cursor-pointer ${
                          type === opt.value
                            ? "border-brand-300 bg-brand-50 text-brand-700"
                            : "border-slate-200 text-slate-600 hover:bg-slate-50"
                        }`}
                      >
                        {opt.label}
                      </button>
                    ))}
                  </div>
                </div>
                <div>
                  <label className="text-xs text-slate-400 mb-1 block">触发词</label>
                  <Textarea
                    value={triggers}
                    onChange={(e) => setTriggers(e.target.value)}
                    rows={3}
                    placeholder="逗号分隔"
                  />
                </div>
                <div>
                  <label className="text-xs text-slate-400 mb-1 block">变更说明（可选）</label>
                  <Input
                    value={changeSummary}
                    onChange={(e) => setChangeSummary(e.target.value)}
                    placeholder="简述本次修改内容"
                  />
                </div>
              </>
            ) : (
              <>
                <div>
                  <p className="text-xs text-slate-400">版本</p>
                  <p className="text-sm font-mono">v{m.version}</p>
                </div>
                <div>
                  <p className="text-xs text-slate-400">类型</p>
                  <Badge>{m.type}</Badge>
                </div>
                {m.triggers && m.triggers.length > 0 && (
                  <div>
                    <p className="text-xs text-slate-400 mb-1">触发词</p>
                    <div className="flex flex-wrap gap-1">
                      {m.triggers.map((t) => (
                        <Badge key={t} variant="outline">{t}</Badge>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* Right: Tabs — README / Tools / Versions */}
        <div className="lg:col-span-2">
          <Tabs defaultValue="readme">
            <TabsList>
              <TabsTrigger value="readme">README</TabsTrigger>
              <TabsTrigger value="tools">工具 ({editing ? tools.length : (m.tools?.length ?? 0)})</TabsTrigger>
              <TabsTrigger value="versions">版本历史</TabsTrigger>
            </TabsList>

            <TabsContent value="readme">
              <Card>
                <CardContent className="pt-6">
                  {editing ? (
                    <Textarea
                      value={readme}
                      onChange={(e) => setReadme(e.target.value)}
                      rows={16}
                      className="font-mono text-sm"
                      placeholder="Markdown 格式的技能说明文档"
                    />
                  ) : skill.readme ? (
                    <MarkdownContent content={skill.readme} />
                  ) : (
                    <p className="text-sm text-slate-400">暂无 README</p>
                  )}
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent value="tools">
              <Card>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle>工具</CardTitle>
                    {editing && (
                      <div className="flex items-center gap-2">
                        <div className="flex rounded-md border border-slate-200 overflow-hidden">
                          <button
                            type="button"
                            onClick={() => {
                              if (toolEditMode === "json") {
                                try {
                                  setTools(JSON.parse(toolsJson))
                                  setToolsJsonError("")
                                } catch { /* keep current tools */ }
                              }
                              setToolEditMode("visual")
                            }}
                            className={`px-2.5 py-1 text-xs cursor-pointer transition-colors ${
                              toolEditMode === "visual" ? "bg-slate-100 text-slate-900" : "text-slate-500 hover:bg-slate-50"
                            }`}
                          >
                            <Eye className="h-3 w-3 inline mr-1" />可视化
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              setToolsJson(JSON.stringify(tools, null, 2))
                              setToolEditMode("json")
                            }}
                            className={`px-2.5 py-1 text-xs cursor-pointer transition-colors ${
                              toolEditMode === "json" ? "bg-slate-100 text-slate-900" : "text-slate-500 hover:bg-slate-50"
                            }`}
                          >
                            <Code className="h-3 w-3 inline mr-1" />JSON
                          </button>
                        </div>
                        {toolEditMode === "visual" && (
                          <Button
                            variant="ghost" size="sm"
                            onClick={() => setTools([...tools, emptyTool()])}
                          >
                            <Plus className="h-3.5 w-3.5" /> 添加
                          </Button>
                        )}
                      </div>
                    )}
                  </div>
                </CardHeader>
                <CardContent>
                  {editing ? (
                    toolEditMode === "json" ? (
                      <div>
                        <Textarea
                          value={toolsJson}
                          onChange={(e) => {
                            setToolsJson(e.target.value)
                            setToolsJsonError("")
                          }}
                          rows={20}
                          className="font-mono text-xs"
                          placeholder="[]"
                        />
                        {toolsJsonError && (
                          <p className="text-xs text-red-500 mt-1">{toolsJsonError}</p>
                        )}
                      </div>
                    ) : (
                      <ToolsVisualEditor tools={tools} onChange={setTools} />
                    )
                  ) : m.tools && m.tools.length > 0 ? (
                    <div className="space-y-1">
                      {m.tools.map((tool) => (
                        <div key={tool.name} className="text-xs p-2 bg-slate-50 rounded">
                          <p className="font-medium font-mono">{tool.name}</p>
                          <p className="text-slate-500 mt-0.5">{tool.description}</p>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-slate-400">暂无工具</p>
                  )}
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent value="versions">
              <Card>
                <CardContent className="pt-6">
                  {name && <SkillVersions skillName={name} currentVersion={m.version} />}
                </CardContent>
              </Card>
            </TabsContent>
          </Tabs>
        </div>
      </div>
    </div>
  )
}

function ToolsVisualEditor({ tools, onChange }: { tools: SkillTool[]; onChange: (t: SkillTool[]) => void }) {
  const update = (index: number, partial: Partial<SkillTool>) => {
    const next = [...tools]
    next[index] = { ...next[index], ...partial }
    onChange(next)
  }

  const remove = (index: number) => {
    onChange(tools.filter((_, i) => i !== index))
  }

  if (tools.length === 0) {
    return <p className="text-sm text-slate-400">暂无工具，点击「添加」创建</p>
  }

  return (
    <div className="space-y-4">
      {tools.map((tool, i) => (
        <ToolEditor key={i} tool={tool} onChange={(t) => update(i, t)} onRemove={() => remove(i)} />
      ))}
    </div>
  )
}

function ToolEditor({
  tool,
  onChange,
  onRemove,
}: {
  tool: SkillTool
  onChange: (partial: Partial<SkillTool>) => void
  onRemove: () => void
}) {
  const [expanded, setExpanded] = useState(!tool.name)
  const [schemaStr, setSchemaStr] = useState(JSON.stringify(tool.inputSchema, null, 2))
  const [schemaError, setSchemaError] = useState("")

  const handleSchemaChange = (value: string) => {
    setSchemaStr(value)
    setSchemaError("")
    try {
      const parsed = JSON.parse(value)
      onChange({ inputSchema: parsed })
    } catch {
      setSchemaError("JSON 格式错误")
    }
  }

  return (
    <div className="border border-slate-200 rounded-lg overflow-hidden">
      <div
        className="flex items-center justify-between px-3 py-2 bg-slate-50 cursor-pointer hover:bg-slate-100 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-xs font-mono font-medium text-slate-800 truncate">
            {tool.name || "(未命名)"}
          </span>
          {tool.executor.type && (
            <Badge variant="outline" className="text-[10px] shrink-0">{tool.executor.type}</Badge>
          )}
        </div>
        <Button variant="ghost" size="sm" onClick={(e) => { e.stopPropagation(); onRemove() }}>
          <Minus className="h-3.5 w-3.5 text-red-500" />
        </Button>
      </div>
      {expanded && (
        <div className="px-3 py-3 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-slate-400 mb-1 block">工具名称</label>
              <Input
                value={tool.name}
                onChange={(e) => onChange({ name: e.target.value })}
                placeholder="tool_name"
                className="font-mono text-xs"
              />
            </div>
            <div>
              <label className="text-xs text-slate-400 mb-1 block">执行类型</label>
              <div className="flex gap-1">
                {(["http", "script", "internal"] as const).map(t => (
                  <button
                    key={t}
                    type="button"
                    onClick={() => onChange({ executor: { ...tool.executor, type: t } })}
                    className={`px-2 py-1 text-xs rounded border transition-colors cursor-pointer ${
                      tool.executor.type === t
                        ? "border-brand-300 bg-brand-50 text-brand-700"
                        : "border-slate-200 text-slate-500 hover:bg-slate-50"
                    }`}
                  >
                    {t}
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div>
            <label className="text-xs text-slate-400 mb-1 block">描述</label>
            <Input
              value={tool.description}
              onChange={(e) => onChange({ description: e.target.value })}
              placeholder="工具功能描述"
            />
          </div>

          {tool.executor.type === "http" && (
            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-2">
                <label className="text-xs text-slate-400 mb-1 block">URL</label>
                <Input
                  value={tool.executor.url ?? ""}
                  onChange={(e) => onChange({ executor: { ...tool.executor, url: e.target.value } })}
                  placeholder="http://localhost:1998/api/..."
                  className="font-mono text-xs"
                />
              </div>
              <div>
                <label className="text-xs text-slate-400 mb-1 block">Method</label>
                <Input
                  value={tool.executor.method ?? "POST"}
                  onChange={(e) => onChange({ executor: { ...tool.executor, method: e.target.value } })}
                  className="font-mono text-xs"
                />
              </div>
            </div>
          )}

          {tool.executor.type === "script" && (
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Command</label>
              <Input
                value={tool.executor.command ?? ""}
                onChange={(e) => onChange({ executor: { ...tool.executor, command: e.target.value } })}
                placeholder="python3 script.py"
                className="font-mono text-xs"
              />
            </div>
          )}

          {tool.executor.type === "internal" && (
            <div>
              <label className="text-xs text-slate-400 mb-1 block">Handler</label>
              <Input
                value={tool.executor.handler ?? ""}
                onChange={(e) => onChange({ executor: { ...tool.executor, handler: e.target.value } })}
                placeholder="handler_name"
                className="font-mono text-xs"
              />
            </div>
          )}

          <div>
            <label className="text-xs text-slate-400 mb-1 block">inputSchema (JSON)</label>
            <Textarea
              value={schemaStr}
              onChange={(e) => handleSchemaChange(e.target.value)}
              rows={6}
              className="font-mono text-xs"
            />
            {schemaError && <p className="text-xs text-red-500 mt-1">{schemaError}</p>}
          </div>
        </div>
      )}
    </div>
  )
}
