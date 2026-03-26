import { AlertTriangle } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  sessionName: string
  onConfirm: () => void
}

export function DeleteSessionDialog({ open, onOpenChange, sessionName, onConfirm }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <div className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-red-100">
              <AlertTriangle className="h-4.5 w-4.5 text-red-600" />
            </div>
            <div>
              <DialogTitle className="text-base">删除会话</DialogTitle>
              <DialogDescription className="mt-0.5">此操作不可恢复</DialogDescription>
            </div>
          </div>
        </DialogHeader>
        <p className="text-sm text-slate-600">
          确定删除会话 <span className="font-medium text-slate-900">{sessionName}</span>？将同时清除数据库记录、执行链路和 Agent 工作目录。
        </p>
        <div className="flex justify-end gap-2 mt-2">
          <Button variant="secondary" size="sm" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button variant="danger" size="sm" onClick={onConfirm}>
            删除
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
