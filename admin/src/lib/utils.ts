import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Format relative time (e.g. "3 分钟前")
 */
export function timeAgo(date: string | number | Date): string {
  const now = Date.now()
  const then = new Date(date).getTime()
  const diff = now - then

  const seconds = Math.floor(diff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (seconds < 60) return "刚刚"
  if (minutes < 60) return `${minutes} 分钟前`
  if (hours < 24) return `${hours} 小时前`
  if (days < 30) return `${days} 天前`
  return new Date(date).toLocaleDateString("zh-CN")
}

/**
 * Format duration in ms to human readable
 */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  const minutes = Math.floor(ms / 60000)
  const seconds = Math.floor((ms % 60000) / 1000)
  return `${minutes}m ${seconds}s`
}

/**
 * Format cost in USD
 */
export function formatCost(usd: number): string {
  if (usd < 0.01) return `$${usd.toFixed(4)}`
  return `$${usd.toFixed(2)}`
}

/**
 * Truncate string with ellipsis
 */
export function truncate(str: string, maxLength: number): string {
  if (str.length <= maxLength) return str
  return str.slice(0, maxLength) + "..."
}

/**
 * Channel display names
 */
export const CHANNEL_LABELS: Record<string, string> = {
  webui: "WebUI",
  feishu: "飞书",
  qiwei: "企微",
}

/**
 * Get channel display name
 */
export function channelLabel(channel: string): string {
  return CHANNEL_LABELS[channel] ?? channel
}

/**
 * Copy text to clipboard with fallback for non-HTTPS contexts.
 * Returns a promise that resolves on success and rejects on failure.
 */
export function copyToClipboard(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    return navigator.clipboard.writeText(text)
  }
  // Fallback: hidden textarea + execCommand
  const textarea = document.createElement("textarea")
  textarea.value = text
  textarea.style.position = "fixed"
  textarea.style.left = "-9999px"
  document.body.appendChild(textarea)
  textarea.select()
  try {
    document.execCommand("copy")
    return Promise.resolve()
  } catch {
    return Promise.reject(new Error("execCommand copy failed"))
  } finally {
    document.body.removeChild(textarea)
  }
}
