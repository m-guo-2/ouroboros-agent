import { Outlet, useLocation } from "react-router-dom"
import { Sidebar } from "./sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"

// Pages that need full-width (no container constraint)
const FULL_WIDTH_PATHS = ["/monitor"]

export function AppLayout() {
  const location = useLocation()
  const isFullWidth = FULL_WIDTH_PATHS.some((p) => location.pathname.startsWith(p))

  return (
    <TooltipProvider>
      <div className="flex h-screen overflow-hidden bg-slate-50">
        <Sidebar />
        <main className={isFullWidth ? "flex-1 overflow-hidden" : "flex-1 overflow-y-auto"}>
          {isFullWidth ? (
            <Outlet />
          ) : (
            <div className="mx-auto max-w-6xl px-6 py-6">
              <Outlet />
            </div>
          )}
        </main>
      </div>
    </TooltipProvider>
  )
}
