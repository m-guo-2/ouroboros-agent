export function safePretty(value: unknown): string {
  if (value == null) return ""
  if (typeof value === "string") {
    const text = value.trim()
    if (!text) return ""
    if ((text.startsWith("{") && text.endsWith("}")) || (text.startsWith("[") && text.endsWith("]"))) {
      try { return JSON.stringify(JSON.parse(text), null, 2) } catch { return value }
    }
    return value
  }
  try { return JSON.stringify(value, null, 2) } catch { return String(value) }
}

export function escapeHtml(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
}

export function openJsonInNewTab(title: string, value: unknown): void {
  const raw = safePretty(value)
  const html = `<!doctype html><html><head><meta charset="utf-8"/><title>${escapeHtml(title)}</title>
<style>body{margin:0;padding:16px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;background:#f8fafc;color:#0f172a}
pre{white-space:pre-wrap;word-break:break-word;background:#fff;border:1px solid #e2e8f0;border-radius:8px;padding:12px}</style>
</head><body><pre>${escapeHtml(raw)}</pre></body></html>`
  const blob = new Blob([html], { type: "text/html;charset=utf-8" })
  const url = URL.createObjectURL(blob)
  window.open(url, "_blank")
  setTimeout(() => URL.revokeObjectURL(url), 60_000)
}
