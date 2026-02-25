import { useState } from "react"
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { useCreateSkill } from "@/hooks/use-skills"
import type { SkillManifest } from "@/api/types"

interface SkillFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const typeOptions = [
  { value: "knowledge", label: "知识" },
  { value: "action", label: "动作" },
  { value: "hybrid", label: "混合" },
] as const

export function SkillFormDialog({ open, onOpenChange }: SkillFormDialogProps) {
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [type, setType] = useState<SkillManifest["type"]>("knowledge")
  const [triggers, setTriggers] = useState("")
  const [readme, setReadme] = useState("")
  const createMutation = useCreateSkill()

  const reset = () => {
    setName("")
    setDescription("")
    setType("knowledge")
    setTriggers("")
    setReadme("")
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !description.trim()) return

    const manifest: Omit<SkillManifest, "version"> = {
      name: name.trim(),
      description: description.trim(),
      type,
      enabled: true,
      triggers: triggers.split(/[,，\n]/).map(s => s.trim()).filter(Boolean),
      tools: [],
    }

    await createMutation.mutateAsync({ name: name.trim(), manifest, readme: readme.trim() || undefined })
    reset()
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>新建技能</DialogTitle>
          <DialogDescription>创建一个新的 Skill，工具可在详情页配置</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">名称</label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="如：feishu-agent"
              autoFocus
            />
          </div>

          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">描述</label>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="简要描述技能的功能"
            />
          </div>

          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">类型</label>
            <div className="flex gap-2">
              {typeOptions.map(opt => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setType(opt.value)}
                  className={`px-3 py-1.5 text-sm rounded-md border transition-colors cursor-pointer ${
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
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">触发词</label>
            <Input
              value={triggers}
              onChange={(e) => setTriggers(e.target.value)}
              placeholder="逗号分隔，如：飞书, Lark, 群聊"
            />
            <p className="text-xs text-slate-400 mt-1">用逗号分隔多个触发词</p>
          </div>

          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">README</label>
            <Textarea
              value={readme}
              onChange={(e) => setReadme(e.target.value)}
              placeholder="技能说明文档（Markdown 格式）"
              rows={4}
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
