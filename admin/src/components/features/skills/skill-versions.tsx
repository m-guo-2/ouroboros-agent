import { useState } from "react"
import { History, RotateCcw, ChevronDown, ChevronRight } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { MarkdownContent } from "@/components/shared/markdown-content"
import { useSkillVersions, useSkillVersion, useRestoreSkillVersion } from "@/hooks/use-skills"

interface SkillVersionsProps {
  skillName: string
  currentVersion: number
}

export function SkillVersions({ skillName, currentVersion }: SkillVersionsProps) {
  const { data: versions, isLoading } = useSkillVersions(skillName)
  const restoreMutation = useRestoreSkillVersion()
  const [expandedVersion, setExpandedVersion] = useState<number | null>(null)

  if (isLoading) {
    return <p className="text-sm text-slate-400">加载中...</p>
  }

  if (!versions || versions.length === 0) {
    return (
      <div className="flex flex-col items-center py-8 text-slate-400">
        <History className="h-8 w-8 mb-2" />
        <p className="text-sm">暂无版本记录</p>
      </div>
    )
  }

  const handleRestore = async (version: number) => {
    if (!confirm(`确认回滚到版本 v${version}？将产生新版本号。`)) return
    await restoreMutation.mutateAsync({ id: skillName, version })
  }

  return (
    <div className="space-y-1">
      {versions.map((v) => (
        <VersionRow
          key={v.version}
          skillName={skillName}
          version={v.version}
          changeSummary={v.changeSummary}
          createdAt={v.createdAt}
          isCurrent={v.version === currentVersion}
          isExpanded={expandedVersion === v.version}
          onToggle={() => setExpandedVersion(expandedVersion === v.version ? null : v.version)}
          onRestore={() => handleRestore(v.version)}
          restoring={restoreMutation.isPending}
        />
      ))}
    </div>
  )
}

function VersionRow({
  skillName,
  version,
  changeSummary,
  createdAt,
  isCurrent,
  isExpanded,
  onToggle,
  onRestore,
  restoring,
}: {
  skillName: string
  version: number
  changeSummary: string
  createdAt: string
  isCurrent: boolean
  isExpanded: boolean
  onToggle: () => void
  onRestore: () => void
  restoring: boolean
}) {
  return (
    <div className="border border-slate-200 rounded-lg overflow-hidden">
      <div
        className="flex items-center justify-between px-3 py-2 cursor-pointer hover:bg-slate-50 transition-colors"
        onClick={onToggle}
      >
        <div className="flex items-center gap-2 min-w-0">
          {isExpanded ? <ChevronDown className="h-3.5 w-3.5 text-slate-400 shrink-0" /> : <ChevronRight className="h-3.5 w-3.5 text-slate-400 shrink-0" />}
          <span className="text-sm font-mono font-medium">v{version}</span>
          {isCurrent && <Badge className="text-[10px]">当前</Badge>}
          {changeSummary && (
            <span className="text-xs text-slate-500 truncate">{changeSummary}</span>
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <span className="text-xs text-slate-400">{formatTime(createdAt)}</span>
          {!isCurrent && (
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-xs"
              onClick={(e) => { e.stopPropagation(); onRestore() }}
              disabled={restoring}
            >
              <RotateCcw className="h-3 w-3 mr-1" />
              回滚
            </Button>
          )}
        </div>
      </div>
      {isExpanded && (
        <VersionDetail skillName={skillName} version={version} />
      )}
    </div>
  )
}

function VersionDetail({ skillName, version }: { skillName: string; version: number }) {
  const { data, isLoading } = useSkillVersion(skillName, version)

  if (isLoading) {
    return <div className="px-3 py-4 text-xs text-slate-400">加载中...</div>
  }

  if (!data) {
    return <div className="px-3 py-4 text-xs text-slate-400">无法加载版本详情</div>
  }

  return (
    <div className="px-3 py-3 border-t border-slate-100 bg-slate-50/50 space-y-3">
      <div className="grid grid-cols-3 gap-3 text-xs">
        <div>
          <p className="text-slate-400">类型</p>
          <p className="font-medium">{data.type}</p>
        </div>
        <div>
          <p className="text-slate-400">工具数</p>
          <p className="font-medium">{data.tools?.length ?? 0}</p>
        </div>
        <div>
          <p className="text-slate-400">触发词</p>
          <p className="font-medium">{data.triggers?.length ?? 0}</p>
        </div>
      </div>
      {data.readme && (
        <div>
          <p className="text-xs text-slate-400 mb-1">README</p>
          <div className="max-h-48 overflow-y-auto bg-white rounded border border-slate-200 p-2">
            <MarkdownContent content={data.readme} />
          </div>
        </div>
      )}
    </div>
  )
}

function formatTime(dateStr: string): string {
  try {
    const d = new Date(dateStr)
    return d.toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
  } catch {
    return dateStr
  }
}
