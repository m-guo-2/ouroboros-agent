import { useState } from "react"
import { Download, Search, Loader2, Check, AlertCircle, Square, CheckSquare, GitBranch } from "lucide-react"
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { ScrollArea } from "@/components/ui/scroll-area"
import { useBrowseImport, useImportSkills } from "@/hooks/use-skills"
import type { BrowseSkillEntry } from "@/api/skills"

interface SkillImportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SkillImportDialog({ open, onOpenChange }: SkillImportDialogProps) {
  const [url, setUrl] = useState("")
  const [skills, setSkills] = useState<BrowseSkillEntry[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [browseCtx, setBrowseCtx] = useState<{ repo: string; branch: string; path: string } | null>(null)
  const [result, setResult] = useState<{ imported: number; failures: string[] } | null>(null)

  const browseMutation = useBrowseImport()
  const importMutation = useImportSkills()

  const reset = () => {
    setUrl("")
    setSkills([])
    setSelected(new Set())
    setBrowseCtx(null)
    setResult(null)
  }

  const handleBrowse = async () => {
    if (!url.trim()) return
    setResult(null)
    setSkills([])
    setBrowseCtx(null)
    try {
      const res = await browseMutation.mutateAsync(url.trim())
      const data = res.data!
      setSkills(data.skills ?? [])
      setBrowseCtx({ repo: data.repo, branch: data.branch, path: data.path })
      setSelected(new Set())
    } catch {
      setSkills([])
      setBrowseCtx(null)
    }
  }

  const toggleSelect = (id: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const selectAll = () => {
    const importable = skills.filter(s => !s.exists).map(s => s.id)
    setSelected(prev =>
      prev.size === importable.length ? new Set() : new Set(importable)
    )
  }

  const handleImport = async () => {
    if (!browseCtx || selected.size === 0) return
    try {
      const res = await importMutation.mutateAsync({
        repo: browseCtx.repo,
        branch: browseCtx.branch,
        path: browseCtx.path,
        skills: Array.from(selected),
      })
      const data = res.data!
      const failures = data.results
        .filter(r => !r.ok)
        .map(r => `${r.id}: ${r.error}`)
      setResult({ imported: data.imported, failures })
      if (data.imported > 0) {
        setSkills(prev => prev.map(s => selected.has(s.id) && data.results.find(r => r.id === s.id)?.ok
          ? { ...s, exists: true } : s
        ))
        setSelected(new Set())
      }
    } catch {
      // error handled by mutation state
    }
  }

  const importableCount = skills.filter(s => !s.exists).length
  const hasBrowsed = browseCtx !== null

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) reset(); onOpenChange(v) }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>导入技能</DialogTitle>
          <DialogDescription>从公开 GitHub 仓库浏览并导入技能到本地仓库</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* URL input */}
          <div>
            <div className="flex gap-2">
              <Input
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://github.com/owner/repo/tree/main/skills"
                onKeyDown={(e) => e.key === "Enter" && handleBrowse()}
                autoFocus
              />
              <Button
                onClick={handleBrowse}
                disabled={!url.trim() || browseMutation.isPending}
                className="shrink-0"
              >
                {browseMutation.isPending
                  ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  : <Search className="h-3.5 w-3.5" />}
                浏览
              </Button>
            </div>
            <p className="text-xs text-slate-400 mt-1.5">
              支持完整 URL（github.com/owner/repo/tree/...）或简写（owner/repo）
            </p>
          </div>

          {browseMutation.isError && (
            <p className="text-sm text-red-600">{(browseMutation.error as Error).message}</p>
          )}

          {/* Loading skeleton */}
          {browseMutation.isPending && (
            <div className="space-y-2 pt-1">
              {[1, 2, 3, 4].map((i) => (
                <div key={i} className="flex items-center gap-3 px-3 py-2.5">
                  <Skeleton className="h-4 w-4 rounded shrink-0" />
                  <div className="flex-1 space-y-1.5">
                    <Skeleton className="h-3.5 w-32" />
                    <Skeleton className="h-3 w-48" />
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Skill list */}
          {!browseMutation.isPending && skills.length > 0 && (
            <>
              {/* Repo context */}
              {browseCtx && (
                <div className="flex items-center gap-2 text-xs text-slate-500">
                  <GitBranch className="h-3 w-3" />
                  <span className="font-mono">{browseCtx.repo}</span>
                  {browseCtx.path && (
                    <>
                      <span className="text-slate-300">/</span>
                      <span className="font-mono">{browseCtx.path}</span>
                    </>
                  )}
                  <Badge className="bg-slate-100 text-slate-500 text-[10px] ml-1">{browseCtx.branch}</Badge>
                </div>
              )}

              <div className="flex items-center justify-between">
                <span className="text-sm text-slate-500">
                  发现 {skills.length} 个技能，{importableCount} 个可导入
                </span>
                {importableCount > 0 && (
                  <Button variant="ghost" size="sm" onClick={selectAll} className="text-xs h-7">
                    {selected.size === importableCount ? "取消全选" : "全选"}
                  </Button>
                )}
              </div>

              <ScrollArea className="max-h-72">
                <div className="space-y-0.5">
                  {skills.map((skill) => {
                    const isSelected = selected.has(skill.id)
                    const Icon = skill.exists ? CheckSquare : isSelected ? CheckSquare : Square
                    return (
                      <div
                        key={skill.id}
                        role="button"
                        tabIndex={skill.exists ? -1 : 0}
                        onClick={() => !skill.exists && toggleSelect(skill.id)}
                        onKeyDown={(e) => e.key === " " && !skill.exists && toggleSelect(skill.id)}
                        className={`flex items-start gap-3 rounded-lg px-3 py-2.5 transition-colors ${
                          skill.exists
                            ? "opacity-45 cursor-default"
                            : isSelected
                              ? "bg-blue-50 cursor-pointer"
                              : "hover:bg-slate-50 cursor-pointer"
                        }`}
                      >
                        <Icon className={`h-4 w-4 mt-0.5 shrink-0 ${
                          skill.exists
                            ? "text-slate-300"
                            : isSelected
                              ? "text-blue-600"
                              : "text-slate-400"
                        }`} />
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium text-slate-900">{skill.name}</span>
                            {skill.name !== skill.id && (
                              <span className="text-xs text-slate-400">{skill.id}</span>
                            )}
                            {skill.exists && (
                              <Badge className="bg-slate-100 text-slate-500 text-[10px]">已存在</Badge>
                            )}
                          </div>
                          {skill.description && (
                            <p className="text-xs text-slate-500 mt-0.5 line-clamp-2">{skill.description}</p>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </ScrollArea>
            </>
          )}

          {hasBrowsed && skills.length === 0 && !browseMutation.isPending && (
            <p className="text-sm text-slate-500 text-center py-4">该路径下未发现技能</p>
          )}

          {/* Result feedback */}
          {result && (
            <div className={`flex items-start gap-2 rounded-lg p-3 text-sm ${
              result.failures.length > 0 ? "bg-amber-50 text-amber-800" : "bg-emerald-50 text-emerald-800"
            }`}>
              {result.failures.length > 0
                ? <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                : <Check className="h-4 w-4 mt-0.5 shrink-0" />}
              <div>
                <p>成功导入 {result.imported} 个技能</p>
                {result.failures.map((f, i) => (
                  <p key={i} className="text-xs mt-1 opacity-80">{f}</p>
                ))}
              </div>
            </div>
          )}

          {/* Actions */}
          {(skills.length > 0 || hasBrowsed) && (
            <div className="flex justify-end gap-2 pt-1">
              <Button variant="secondary" onClick={() => onOpenChange(false)}>关闭</Button>
              {importableCount > 0 && (
                <Button
                  onClick={handleImport}
                  disabled={selected.size === 0 || importMutation.isPending}
                >
                  {importMutation.isPending
                    ? <><Loader2 className="h-3.5 w-3.5 animate-spin" />正在导入 {selected.size} 个技能…</>
                    : <><Download className="h-3.5 w-3.5" />导入{selected.size > 0 ? ` (${selected.size})` : ""}</>}
                </Button>
              )}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
