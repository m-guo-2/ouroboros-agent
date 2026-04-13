import { useState } from "react"
import { Link } from "react-router-dom"
import { Blocks, Plus, Pencil, RefreshCw, FileCode, FileText, Download } from "lucide-react"
import { PageHeader } from "@/components/layout/page-header"
import { EmptyState } from "@/components/layout/empty-state"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { SkillFormDialog } from "./skill-form"
import { SkillImportDialog } from "./skill-import-dialog"
import { useSkills, useToggleSkill, useRefreshSkills } from "@/hooks/use-skills"

const AVATAR_COLORS = [
  "bg-blue-100 text-blue-700",
  "bg-violet-100 text-violet-700",
  "bg-amber-100 text-amber-700",
  "bg-emerald-100 text-emerald-700",
  "bg-rose-100 text-rose-700",
  "bg-cyan-100 text-cyan-700",
  "bg-orange-100 text-orange-700",
  "bg-teal-100 text-teal-700",
]

function nameToColor(name: string) {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]
}

function nameToInitial(name: string) {
  return name.charAt(0).toUpperCase()
}

export function SkillList() {
  const { data: skills, isLoading } = useSkills()
  const toggleMutation = useToggleSkill()
  const refreshMutation = useRefreshSkills()
  const [showCreate, setShowCreate] = useState(false)
  const [showImport, setShowImport] = useState(false)

  if (isLoading) {
    return (
      <div>
        <PageHeader title="Skills" description="管理 Agent 技能" />
        <div className="mt-6 space-y-1.5">
          {[1, 2, 3, 4, 5].map((i) => <Skeleton key={i} className="h-12 rounded-lg" />)}
        </div>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Skills"
        description="管理 Agent 技能"
        actions={
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => refreshMutation.mutate()}
              disabled={refreshMutation.isPending}
            >
              <RefreshCw className={`h-3.5 w-3.5 ${refreshMutation.isPending ? "animate-spin" : ""}`} />
              {refreshMutation.isPending ? "同步中…" : "同步仓库"}
            </Button>
            <Button variant="secondary" size="sm" onClick={() => setShowImport(true)}>
              <Download className="h-3.5 w-3.5" />
              导入
            </Button>
            <Button size="sm" onClick={() => setShowCreate(true)}>
              <Plus className="h-3.5 w-3.5" />
              新建技能
            </Button>
          </div>
        }
      />

      <SkillFormDialog open={showCreate} onOpenChange={setShowCreate} />
      <SkillImportDialog open={showImport} onOpenChange={setShowImport} />

      {!skills || skills.length === 0 ? (
        <EmptyState icon={Blocks} title="暂无技能" className="mt-12" />
      ) : (
        <Card className="mt-6">
          <div className="divide-y divide-slate-100">
            {skills.map((skill) => {
              const scriptCount = skill.scripts?.length ?? 0
              const refCount = skill.references?.length ?? 0
              return (
                <div
                  key={skill.id}
                  className="flex items-center gap-3 px-4 py-2.5 hover:bg-slate-50/80 transition-colors"
                >
                  <Link to={`/skills/${skill.id}`} className="flex items-center gap-3 flex-1 min-w-0">
                    <div className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-xs font-semibold ${nameToColor(skill.name)}`}>
                      {nameToInitial(skill.name)}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-slate-900 truncate">{skill.name}</span>
                        <div className="flex items-center gap-1 shrink-0">
                          {scriptCount > 0 && (
                            <Badge className="bg-emerald-50 text-emerald-700 gap-0.5 text-[10px] px-1.5 py-0">
                              <FileCode className="h-2.5 w-2.5" />{scriptCount}
                            </Badge>
                          )}
                          {refCount > 0 && (
                            <Badge className="bg-blue-50 text-blue-700 gap-0.5 text-[10px] px-1.5 py-0">
                              <FileText className="h-2.5 w-2.5" />{refCount}
                            </Badge>
                          )}
                        </div>
                      </div>
                      {skill.description && (
                        <p className="text-xs text-slate-400 truncate">{skill.description}</p>
                      )}
                    </div>
                  </Link>

                  <div className="flex items-center gap-1.5 shrink-0">
                    <Link to={`/skills/${skill.id}?edit=1`}>
                      <Button variant="ghost" size="sm" className="h-7 w-7 p-0" title="编辑">
                        <Pencil className="h-3 w-3 text-slate-400" />
                      </Button>
                    </Link>
                    <Switch
                      checked={skill.enabled}
                      onCheckedChange={(enabled) => toggleMutation.mutate({ id: skill.id, enabled })}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </Card>
      )}
    </div>
  )
}
