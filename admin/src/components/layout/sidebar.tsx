import { NavLink, useLocation } from "react-router-dom"
import {
  Bot, Blocks, Cpu,
  Activity,
  Settings,
  PanelLeftClose, PanelLeft,
} from "lucide-react"
import { cn } from "@/lib/utils"
import { useUIStore } from "@/stores/ui-store"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

interface NavItem {
  label: string
  path: string
  icon: React.ComponentType<{ className?: string }>
  live?: boolean
}

const navGroups: { title: string; items: NavItem[] }[] = [
  {
    title: "管理",
    items: [
      { label: "Agents", path: "/agents", icon: Bot },
      { label: "Skills", path: "/skills", icon: Blocks },
      { label: "Models", path: "/models", icon: Cpu },
    ],
  },
  {
    title: "观测",
    items: [
      { label: "Monitor", path: "/monitor", icon: Activity, live: true },
    ],
  },
  {
    title: "系统",
    items: [
      { label: "Settings", path: "/settings", icon: Settings },
    ],
  },
]

function SidebarNavItem({ item, collapsed }: { item: NavItem; collapsed: boolean }) {
  const location = useLocation()
  const isActive = location.pathname === item.path || location.pathname.startsWith(item.path + "/")

  const linkContent = (
    <NavLink
      to={item.path}
      className={cn(
        "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors duration-150",
        collapsed && "justify-center px-0",
        isActive
          ? "bg-brand-50 text-brand-700"
          : "text-slate-600 hover:bg-slate-100 hover:text-slate-900"
      )}
    >
      <span className="relative flex-shrink-0">
        <item.icon className={cn("h-[18px] w-[18px]", isActive ? "text-brand-600" : "")} />
        {item.live && (
          <span className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-green-500 animate-live-pulse" />
        )}
      </span>
      {!collapsed && <span>{item.label}</span>}
    </NavLink>
  )

  if (collapsed) {
    return (
      <Tooltip delayDuration={0}>
        <TooltipTrigger asChild>{linkContent}</TooltipTrigger>
        <TooltipContent side="right">{item.label}</TooltipContent>
      </Tooltip>
    )
  }

  return linkContent
}

export function Sidebar() {
  const collapsed = useUIStore((s) => s.sidebarCollapsed)
  const toggle = useUIStore((s) => s.toggleSidebar)

  return (
    <aside
      className={cn(
        "flex flex-col border-r border-slate-200 bg-white transition-all duration-200",
        collapsed ? "w-14" : "w-60"
      )}
    >
      {/* Logo / header */}
      <div className={cn(
        "flex h-14 items-center border-b border-slate-100 px-4",
        collapsed && "justify-center px-2"
      )}>
        {!collapsed && (
          <span className="text-sm font-bold tracking-tight text-slate-900">
            Ouroboros
          </span>
        )}
        {collapsed && (
          <span className="text-sm font-bold text-brand-600">O</span>
        )}
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto px-2 py-3">
        {navGroups.map((group) => (
          <div key={group.title} className="mb-4">
            {!collapsed && (
              <p className="mb-1.5 px-3 text-[11px] font-medium uppercase tracking-wider text-slate-400">
                {group.title}
              </p>
            )}
            {collapsed && <div className="mb-1.5" />}
            <div className="space-y-0.5">
              {group.items.map((item) => (
                <SidebarNavItem key={item.path} item={item} collapsed={collapsed} />
              ))}
            </div>
          </div>
        ))}
      </nav>

      {/* Collapse toggle */}
      <div className="border-t border-slate-100 p-2">
        <button
          onClick={toggle}
          className={cn(
            "flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-slate-400 hover:bg-slate-100 hover:text-slate-600 transition-colors cursor-pointer",
            collapsed && "justify-center px-0"
          )}
        >
          {collapsed ? <PanelLeft className="h-[18px] w-[18px]" /> : <PanelLeftClose className="h-[18px] w-[18px]" />}
          {!collapsed && <span>收起</span>}
        </button>
      </div>
    </aside>
  )
}
