import { useState } from "react"
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { useCreateSkill } from "@/hooks/use-skills"

interface SkillFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SkillFormDialog({ open, onOpenChange }: SkillFormDialogProps) {
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [readme, setReadme] = useState("")
  const createMutation = useCreateSkill()

  const reset = () => {
    setName("")
    setDescription("")
    setReadme("")
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !description.trim()) return

    await createMutation.mutateAsync({
      name: name.trim(),
      description: description.trim(),
      enabled: true,
      readme: readme.trim() || undefined,
    })
    reset()
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>新建技能</DialogTitle>
          <DialogDescription>创建一个新的 Skill，脚本和参考文档在 GitHub 仓库中管理</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">名称</label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="如：code-review"
              autoFocus
            />
          </div>

          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">描述</label>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="描述技能的功能和适用场景（LLM 匹配触发的唯一依据）"
            />
          </div>

          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">SKILL.md 内容</label>
            <Textarea
              value={readme}
              onChange={(e) => setReadme(e.target.value)}
              placeholder="Markdown 格式的操作指令，包含脚本用法、参数说明等"
              rows={6}
            />
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="secondary" type="button" onClick={() => onOpenChange(false)}>取消</Button>
            <Button type="submit" disabled={!name.trim() || !description.trim() || createMutation.isPending}>
              {createMutation.isPending ? "创建中..." : "创建"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
