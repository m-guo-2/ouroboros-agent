import { fetchApi } from "./client"

// QiweiAccount mirrors channel-qiwei's maskedAccount JSON shape. The raw
// token is never returned by the backend; only a short preview (e.g.
// "abcd***") comes back as `tokenPreview` so it's safe to render in a list.
export interface QiweiAccount {
  id: string
  guid: string
  token_preview: string
  shortHash: string
  displayName: string
  agentId: string
  enabled: boolean
  selfUserId: string
  selfName: string
  selfAlias: string
  selfAvatarUrl: string
  selfCorpName: string
  selfSyncedAt: number
  metaJson: string
  notes: string
  createdAt: number
  updatedAt: number

  // contact gateway stats — only populated on the detail endpoint.
  contact_count?: number
  room_count?: number
  room_member_count?: number
  identity_link_count?: number
  contact_sync_last_at?: number
  contact_sync_last_error?: string
}

export interface CreateQiweiAccountInput {
  guid: string
  token: string
  displayName?: string
  agentId?: string
  enabled?: boolean
  metaJson?: string
  notes?: string
}

export interface UpdateQiweiAccountInput {
  token?: string
  displayName?: string
  agentId?: string
  enabled?: boolean
  metaJson?: string
  notes?: string
}

export interface UnknownGuidEvent {
  guid: string
  cmd: number
  msgType: number
  msgSvrId?: string
  at: number
}

const base = "/qiwei/_admin"

export const qiweiAccountsApi = {
  list: () => fetchApi<QiweiAccount[]>(`${base}/accounts`),

  get: (id: string) => fetchApi<QiweiAccount>(`${base}/accounts/${id}`),

  create: (data: CreateQiweiAccountInput) =>
    fetchApi<QiweiAccount>(`${base}/accounts`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  update: (id: string, data: UpdateQiweiAccountInput) =>
    fetchApi<QiweiAccount>(`${base}/accounts/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),

  softDelete: (id: string) =>
    fetchApi<void>(`${base}/accounts/${id}`, { method: "DELETE" }),

  hardDelete: (id: string) =>
    fetchApi<void>(`${base}/accounts/${id}/hard_delete`, { method: "POST" }),

  refreshProfile: (id: string) =>
    fetchApi<QiweiAccount>(`${base}/accounts/${id}/refresh`, { method: "POST" }),

  reload: () => fetchApi<{ count: number }>(`${base}/reload`, { method: "POST" }),

  unknownGuids: () => fetchApi<UnknownGuidEvent[]>(`${base}/unknown_guids`),
}
