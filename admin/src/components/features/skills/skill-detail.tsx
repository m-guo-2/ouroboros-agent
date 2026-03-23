import { useState, useEffect, useCallback, useRef } from "react"
import { useParams, Link, useNavigate, useSearchParams } from "react-router-dom"
import { ArrowLeft, Trash2, Pencil, Save, X } from "lucide-react"
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
import { useSkill, useToggleSkill, useDeleteSkill, useUpdateSkill } from "@/hooks/use-skills"

export function SkillDetail() {
  const { name: skillId } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const { data: skill, isLoading } = useSkill(skillId)
  const toggleMutation = useToggleSkill()
  const deleteMutation = useDeleteSkill()
  const updateMutation = useUpdateSkill()

  const [editing, setEditing] = useState(false)
  const [description, setDescription] = useState("")
  const [readme, setReadme] = useState("")

  const syncFromSkill = useCallback(() => {
    if (!skill) return
    setDescription(skill.description ?? "")
    setReadme(skill.readme ?? "")
  }, [skill])

  useEffect(() => { syncFromSkill() }, [syncFromSkill])

  const appliedEditParam = useRef(false)
  const [searchParams] = useSearchParams()
  useEffect(() => {
    if (searchParams.get("edit") === "1" && skill && !appliedEditParam.current) {
      appliedEditParam.current = true
      syncFromSkill()
      setEditing(true)
    }
  }, [searchParams.get("edit"), skill, syncFromSkill])

  const enterEdit = () => { syncFromSkill(); setEditing(true) }
  const cancelEdit = () => { syncFromSkill(); setEditing(false) }

  const handleSave = async () => {
    if (!skillId) return
    await updateMutation.mutateAsync({
      id: skillId,
      description: description.trim(),
      readme: readme.trim(),
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

  return (
    <div>
      <PageHeader
        title={skill.name}
        description={editing ? undefined : skill.description}
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
                  checked={skill.enabled}
                  onCheckedChange={(enabled) => toggleMutation.mutate({ id: skill.id, enabled })}
                />
                <Button variant="secondary" size="sm" onClick={enterEdit}>
                  <Pencil className="h-4 w-4" /> 编辑
                </Button>
                <Button
                  variant="ghost" size="sm"
                  onClick={async () => {
                    if (!confirm("确认删除？")) return
                    await deleteMutation.mutateAsync(skill.id)
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
        <Card className="lg:col-span-1">
          <CardHeader><CardTitle>信息</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            {editing ? (
              <div>
                <label className="text-xs text-slate-400 mb-1 block">描述</label>
                <Input value={description} onChange={(e) => setDescription(e.target.value)} />
              </div>
            ) : (
              <>
                <div>
                  <p className="text-xs text-slate-400">ID</p>
                  <p className="text-sm font-mono">{skill.id}</p>
                </div>
                <div>
                  <p className="text-xs text-slate-400">状态</p>
                  <Badge variant={skill.enabled ? "brand" : "outline"}>
                    {skill.enabled ? "启用" : "禁用"}
                  </Badge>
                </div>
                {skill.scripts && skill.scripts.length > 0 && (
                  <div>
                    <p className="text-xs text-slate-400 mb-1">脚本</p>
                    <div className="flex flex-wrap gap-1">
                      {skill.scripts.map((s: string) => (
                        <Badge key={s} variant="outline" className="font-mono text-xs">{s}</Badge>
                      ))}
                    </div>
                  </div>
                )}
                {skill.references && skill.references.length > 0 && (
                  <div>
                    <p className="text-xs text-slate-400 mb-1">参考文档</p>
                    <div className="flex flex-wrap gap-1">
                      {skill.references.map((r: string) => (
                        <Badge key={r} variant="outline" className="text-xs">{r}</Badge>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

        <div className="lg:col-span-2">
          <Tabs defaultValue="readme">
            <TabsList>
              <TabsTrigger value="readme">SKILL.md</TabsTrigger>
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
                    <p className="text-sm text-slate-400">暂无内容</p>
                  )}
                </CardContent>
              </Card>
            </TabsContent>
          </Tabs>
        </div>
      </div>
    </div>
  )
}
