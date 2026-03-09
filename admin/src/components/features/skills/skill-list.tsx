import { useState } from "react"
import { Link } from "react-router-dom"
import { Blocks, Wrench, BookOpen, Zap, Plus, Pencil, Cpu, Download } from "lucide-react"
import { PageHeader } from "@/components/layout/page-header"
import { EmptyState } from "@/components/layout/empty-state"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { SkillFormDialog } from "./skill-form"
import { useSkills, useToggleSkill } from "@/hooks/use-skills"

const typeConfig: Record<string, { icon: React.ComponentType<{ className?: string }>; label: string; color: string }> = {
  knowledge: { icon: BookOpen, label: "知识", color: "bg-blue-50 text-blue-700" },
  action: { icon: Zap, label: "动作", color: "bg-amber-50 text-amber-700" },
  hybrid: { icon: Blocks, label: "混合", color: "bg-purple-50 text-purple-700" },
}

export function SkillList() {
  const { data: skills, isLoading } = useSkills()
  const toggleMutation = useToggleSkill()
  const [showCreate, setShowCreate] = useState(false)

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

  const hasTools = (skill: typeof skills extends (infer T)[] | undefined ? T : never) =>
    Array.isArray(skill.tools) && skill.tools.length > 0

  return (
    <div>
      <PageHeader
        title="Skills"
        description="管理 Agent 技能"
        actions={
          <Button size="sm" onClick={() => setShowCreate(true)}>
            <Plus className="h-3.5 w-3.5" />
            新建技能
          </Button>
        }
      />

      <SkillFormDialog open={showCreate} onOpenChange={setShowCreate} />

      {!skills || skills.length === 0 ? (
        <EmptyState icon={Blocks} title="暂无技能" className="mt-12" />
      ) : (
        <Card className="mt-6">
          <div className="divide-y divide-slate-100">
            {skills.map((skill) => {
              const config = typeConfig[skill.type] ?? typeConfig.hybrid
              const TypeIcon = config.icon
              const isSystem = hasTools(skill)
              return (
                <div
                  key={skill.id}
                  className="flex items-center justify-between px-4 py-3 hover:bg-slate-50 transition-colors"
                >
                  <Link to={`/skills/${skill.id}`} className="flex-1 min-w-0">
                    <div className="flex items-center gap-3">
                      <div className="flex h-8 w-8 items-center justify-center rounded-md bg-slate-100">
                        <TypeIcon className="h-4 w-4 text-slate-600" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium text-slate-900">{skill.name}</span>
                          <Badge className={config.color}>{config.label}</Badge>
                          {isSystem ? (
                            <Badge className="bg-emerald-50 text-emerald-700 gap-0.5">
                              <Cpu className="h-2.5 w-2.5" />系统加载
                            </Badge>
                          ) : (
                            <Badge className="bg-slate-100 text-slate-500 gap-0.5">
                              <Download className="h-2.5 w-2.5" />按需加载
                            </Badge>
                          )}
                          <span className="text-xs text-slate-400">v{skill.version}</span>
                        </div>
                        <p className="text-xs text-slate-500 truncate mt-0.5">{skill.description}</p>
                      </div>
                    </div>
                  </Link>

                  <div className="flex items-center gap-3 shrink-0 ml-4">
                    {isSystem && (
                      <span className="flex items-center gap-1 text-xs text-slate-400">
                        <Wrench className="h-3 w-3" />
                        {skill.tools.length}
                      </span>
                    )}
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
