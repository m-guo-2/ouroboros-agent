import { useState, useEffect, useCallback, useRef } from "react"
import { useParams, Link, useNavigate, useSearchParams } from "react-router-dom"
import { ArrowLeft, Trash2, Pencil, Save, X, FileCode, FileText } from "lucide-react"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
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
        <Skeleton className="h-[calc(100vh-12rem)] mt-4 rounded-lg" />
      </div>
    )
  }

  if (!skill) {
    return <div className="text-sm text-slate-500">技能未找到</div>
  }

  const scriptCount = skill.scripts?.length ?? 0
  const refCount = skill.references?.length ?? 0

  return (
    <div className="flex flex-col h-[calc(100vh-3rem)]">
      {/* Header: back + title + meta + actions */}
      <div className="flex items-center justify-between gap-4 pb-4 shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          <Link to="/skills">
            <Button variant="ghost" size="sm" className="h-7 w-7 p-0 shrink-0">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div className="min-w-0">
            <div className="flex items-center gap-2.5">
              <h1 className="text-lg font-semibold text-slate-900 truncate">{skill.name}</h1>
              <Badge variant={skill.enabled ? "brand" : "outline"} className="shrink-0">
                {skill.enabled ? "启用" : "禁用"}
              </Badge>
            </div>
            {!editing && skill.description && (
              <p className="text-sm text-slate-500 truncate mt-0.5">{skill.description}</p>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {editing ? (
            <>
              <Button variant="secondary" size="sm" onClick={cancelEdit}>
                <X className="h-3.5 w-3.5" /> 取消
              </Button>
              <Button size="sm" onClick={handleSave} disabled={updateMutation.isPending}>
                <Save className="h-3.5 w-3.5" /> {updateMutation.isPending ? "保存中..." : "保存"}
              </Button>
            </>
          ) : (
            <>
              <Switch
                checked={skill.enabled}
                onCheckedChange={(enabled) => toggleMutation.mutate({ id: skill.id, enabled })}
              />
              <Button variant="secondary" size="sm" onClick={enterEdit}>
                <Pencil className="h-3.5 w-3.5" /> 编辑
              </Button>
              <Button
                variant="ghost" size="sm"
                onClick={async () => {
                  if (!confirm("确认删除？")) return
                  await deleteMutation.mutateAsync(skill.id)
                  navigate("/skills")
                }}
              >
                <Trash2 className="h-3.5 w-3.5 text-red-500" />
              </Button>
            </>
          )}
        </div>
      </div>

      {/* Metadata bar: compact inline chips */}
      {!editing && (scriptCount > 0 || refCount > 0) && (
        <div className="flex items-center gap-3 pb-4 shrink-0">
          {scriptCount > 0 && (
            <div className="flex items-center gap-1.5">
              <FileCode className="h-3.5 w-3.5 text-slate-400" />
              <div className="flex flex-wrap gap-1">
                {skill.scripts!.map((s: string) => (
                  <Badge key={s} variant="outline" className="font-mono text-xs">{s}</Badge>
                ))}
              </div>
            </div>
          )}
          {refCount > 0 && (
            <div className="flex items-center gap-1.5">
              <FileText className="h-3.5 w-3.5 text-slate-400" />
              <div className="flex flex-wrap gap-1">
                {skill.references!.map((r: string) => (
                  <Badge key={r} variant="outline" className="text-xs">{r}</Badge>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Edit: description field */}
      {editing && (
        <div className="pb-3 shrink-0">
          <Input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="技能描述"
            className="text-sm"
          />
        </div>
      )}

      {/* Content: full-width, fills remaining vertical space */}
      <Card className="flex-1 min-h-0 flex flex-col">
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-slate-100 shrink-0">
          <span className="text-xs font-medium text-slate-500 uppercase tracking-wide">SKILL.md</span>
        </div>
        <CardContent className="flex-1 min-h-0 overflow-y-auto p-4">
          {editing ? (
            <Textarea
              value={readme}
              onChange={(e) => setReadme(e.target.value)}
              className="font-mono text-sm h-full min-h-[400px] resize-none"
              placeholder="Markdown 格式的技能说明文档"
            />
          ) : skill.readme ? (
            <MarkdownContent content={skill.readme} />
          ) : (
            <p className="text-sm text-slate-400">暂无内容</p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
