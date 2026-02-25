import { Badge } from "@/components/ui/badge"

const statusConfig: Record<string, { label: string; variant: "success" | "brand" | "warning" | "danger" | "default" }> = {
  running: { label: "运行中", variant: "success" },
  active: { label: "活跃", variant: "success" },
  processing: { label: "执行中", variant: "brand" },
  completed: { label: "已完成", variant: "default" },
  idle: { label: "空闲", variant: "default" },
  stopped: { label: "已停止", variant: "default" },
  starting: { label: "启动中", variant: "warning" },
  interrupted: { label: "已中断", variant: "warning" },
  error: { label: "错误", variant: "danger" },
}

export function StatusBadge({ status }: { status: string }) {
  const config = statusConfig[status] ?? { label: status, variant: "default" as const }
  return <Badge variant={config.variant}>{config.label}</Badge>
}
