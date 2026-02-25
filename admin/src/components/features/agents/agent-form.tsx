import { useState } from "react"
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { useCreateAgent } from "@/hooks/use-agents"

interface AgentFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AgentFormDialog({ open, onOpenChange }: AgentFormDialogProps) {
  const [name, setName] = useState("")
  const [prompt, setPrompt] = useState("")
  const createMutation = useCreateAgent()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    await createMutation.mutateAsync({
      displayName: name.trim(),
      systemPrompt: prompt.trim() || undefined,
    })
    setName("")
    setPrompt("")
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新建 Agent</DialogTitle>
          <DialogDescription>创建一个新的 AI Agent 配置</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">名称</label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="如：客服助手"
              autoFocus
            />
          </div>
          <div>
            <label className="text-sm font-medium text-slate-700 mb-1.5 block">系统提示词</label>
            <Textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="定义 Agent 的行为和角色..."
              rows={4}
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="secondary" type="button" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={!name.trim() || createMutation.isPending}>
              {createMutation.isPending ? "创建中..." : "创建"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
