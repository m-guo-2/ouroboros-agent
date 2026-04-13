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
        <div className="mt-6 space-y-2">
          {[1, 2, 3].map((i) => <Skeleton key={i} className="h-16 rounded-lg" />)}
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
                  className="flex items-center justify-between px-4 py-3 hover:bg-slate-50 transition-colors"
                >
                  <Link to={`/skills/${skill.id}`} className="flex-1 min-w-0">
                    <div className="flex items-center gap-3">
                      <div className="flex h-8 w-8 items-center justify-center rounded-md bg-slate-100">
                        <Blocks className="h-4 w-4 text-slate-600" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium text-slate-900">{skill.name}</span>
                          {scriptCount > 0 && (
                            <Badge className="bg-emerald-50 text-emerald-700 gap-0.5">
                              <FileCode className="h-2.5 w-2.5" />{scriptCount} 脚本
                            </Badge>
                          )}
                          {refCount > 0 && (
                            <Badge className="bg-blue-50 text-blue-700 gap-0.5">
                              <FileText className="h-2.5 w-2.5" />{refCount} 参考
                            </Badge>
                          )}
                        </div>
                        <p className="text-xs text-slate-500 truncate mt-0.5">{skill.description}</p>
                      </div>
                    </div>
                  </Link>

                  <div className="flex items-center gap-3 shrink-0 ml-4">
                    <Link to={`/skills/${skill.id}?edit=1`}>
                      <Button variant="ghost" size="sm" title="编辑">
                        <Pencil className="h-3.5 w-3.5 text-slate-500" />
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
