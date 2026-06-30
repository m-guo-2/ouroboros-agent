import { fetchApi } from "./client"
import type { SandboxTemplate } from "./types"

export const sandboxTemplatesApi = {
  list: () => fetchApi<SandboxTemplate[]>("/sandbox-templates"),
}
