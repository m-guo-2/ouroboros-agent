/**
 * Base API client — thin fetch wrapper with typed responses
 */

const API_BASE = "/api"

export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: string
  message?: string
}

// ===== WebUI user identity (localStorage UUID) =====

const WEBUI_USER_KEY = "webui_user_id"

export function getWebuiUserId(): string {
  let userId = localStorage.getItem(WEBUI_USER_KEY)
  if (!userId) {
    userId = `webui-${crypto.randomUUID()}`
    localStorage.setItem(WEBUI_USER_KEY, userId)
  }
  return userId
}

// ===== Core fetch helper =====

export async function fetchApi<T>(
  path: string,
  options?: RequestInit
): Promise<ApiResponse<T>> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: "Request failed" }))
    return { success: false, error: error.error || response.statusText }
  }

  return response.json()
}
