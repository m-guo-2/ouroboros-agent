import { useState, useEffect } from "react"

export function useTimeAgoTick() {
  const [tick, setTick] = useState(0)
  useEffect(() => {
    let id: ReturnType<typeof setInterval> | undefined
    const start = () => { id = setInterval(() => setTick(t => t + 1), 60_000) }
    const stop = () => { if (id) { clearInterval(id); id = undefined } }
    const onVisChange = () => { document.hidden ? stop() : start() }
    if (!document.hidden) start()
    document.addEventListener("visibilitychange", onVisChange)
    return () => { stop(); document.removeEventListener("visibilitychange", onVisChange) }
  }, [])
  return tick
}
