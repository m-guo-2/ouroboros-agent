import { lazy, Suspense } from "react"
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { AppLayout } from "@/components/layout/app-layout"
import { Skeleton } from "@/components/ui/skeleton"

// Lazy-load all pages for code splitting
const MonitorPage = lazy(() => import("@/components/features/monitor/monitor-page").then((m) => ({ default: m.MonitorPage })))
const AgentList = lazy(() => import("@/components/features/agents/agent-list").then((m) => ({ default: m.AgentList })))
const AgentDetail = lazy(() => import("@/components/features/agents/agent-detail").then((m) => ({ default: m.AgentDetail })))
const ModelList = lazy(() => import("@/components/features/models/model-list").then((m) => ({ default: m.ModelList })))
const SkillList = lazy(() => import("@/components/features/skills/skill-list").then((m) => ({ default: m.SkillList })))
const SkillDetail = lazy(() => import("@/components/features/skills/skill-detail").then((m) => ({ default: m.SkillDetail })))
const LogViewer = lazy(() => import("@/components/features/logs/log-viewer").then((m) => ({ default: m.LogViewer })))
const SettingsPage = lazy(() => import("@/components/features/settings/settings-page").then((m) => ({ default: m.SettingsPage })))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

function PageFallback() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-8 w-48" />
      <Skeleton className="h-4 w-64" />
      <Skeleton className="h-64 rounded-lg mt-6" />
    </div>
  )
}

// Error boundary
import { Component, type ReactNode, type ErrorInfo } from "react"

interface ErrorBoundaryProps { children: ReactNode }
interface ErrorBoundaryState { error: Error | null }

class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }
  static getDerivedStateFromError(error: Error) { return { error } }
  componentDidCatch(error: Error, info: ErrorInfo) { console.error("Page error:", error, info) }
  render() {
    if (this.state.error) {
      return (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <div className="h-12 w-12 rounded-xl bg-red-100 flex items-center justify-center mb-4">
            <span className="text-red-600 text-lg font-bold">!</span>
          </div>
          <h2 className="text-sm font-semibold text-slate-900">页面加载出错</h2>
          <p className="text-sm text-slate-500 mt-1 max-w-md">{this.state.error.message}</p>
          <button
            className="mt-4 px-4 py-2 text-sm font-medium text-brand-600 hover:bg-brand-50 rounded-md transition-colors cursor-pointer"
            onClick={() => { this.setState({ error: null }); window.location.reload() }}
          >
            重新加载
          </button>
        </div>
      )
    }
    return this.props.children
  }
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route element={<AppLayout />}>
            <Route index element={<Navigate to="/monitor" replace />} />
            <Route path="monitor" element={<ErrorBoundary><Suspense fallback={<PageFallback />}><MonitorPage /></Suspense></ErrorBoundary>} />
            <Route path="agents" element={<ErrorBoundary><Suspense fallback={<PageFallback />}><AgentList /></Suspense></ErrorBoundary>} />
            <Route path="agents/:id" element={<ErrorBoundary><Suspense fallback={<PageFallback />}><AgentDetail /></Suspense></ErrorBoundary>} />
            <Route path="models" element={<ErrorBoundary><Suspense fallback={<PageFallback />}><ModelList /></Suspense></ErrorBoundary>} />
            <Route path="skills" element={<ErrorBoundary><Suspense fallback={<PageFallback />}><SkillList /></Suspense></ErrorBoundary>} />
            <Route path="skills/:name" element={<ErrorBoundary><Suspense fallback={<PageFallback />}><SkillDetail /></Suspense></ErrorBoundary>} />
            <Route path="logs" element={<ErrorBoundary><Suspense fallback={<PageFallback />}><LogViewer /></Suspense></ErrorBoundary>} />
            <Route path="settings" element={<ErrorBoundary><Suspense fallback={<PageFallback />}><SettingsPage /></Suspense></ErrorBoundary>} />
            <Route path="*" element={<Navigate to="/monitor" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
