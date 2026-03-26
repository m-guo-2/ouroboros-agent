import { useCallback } from "react"
import { useSearchParams } from "react-router-dom"
import type { MonitorTab } from "@/components/features/monitor/monitor-page"

const VALID_TABS: MonitorTab[] = ["conversation", "memory", "tasks", "subagent"]

function isValidTab(v: string | null): v is MonitorTab {
  return !!v && (VALID_TABS as string[]).includes(v)
}

export function useMonitorSearchParams() {
  const [searchParams, setSearchParams] = useSearchParams()

  const sessionId = searchParams.get("session")
  const tab = isValidTab(searchParams.get("tab")) ? (searchParams.get("tab") as MonitorTab) : "conversation"
  const exchangeIndex = searchParams.get("exchange") ? Number(searchParams.get("exchange")) : null

  const setSessionId = useCallback((id: string | null) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (id) {
        next.set("session", id)
      } else {
        next.delete("session")
      }
      next.delete("exchange")
      return next
    }, { replace: true })
  }, [setSearchParams])

  const setTab = useCallback((t: MonitorTab) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (t === "conversation") {
        next.delete("tab")
      } else {
        next.set("tab", t)
      }
      return next
    }, { replace: true })
  }, [setSearchParams])

  const setExchangeIndex = useCallback((idx: number | null) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (idx != null) {
        next.set("exchange", String(idx))
      } else {
        next.delete("exchange")
      }
      return next
    }, { replace: true })
  }, [setSearchParams])

  const clearInvalidSession = useCallback(() => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.delete("session")
      next.delete("exchange")
      return next
    }, { replace: true })
  }, [setSearchParams])

  return {
    sessionId,
    tab,
    exchangeIndex,
    setSessionId,
    setTab,
    setExchangeIndex,
    clearInvalidSession,
  }
}
