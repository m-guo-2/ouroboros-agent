import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  qiweiAccountsApi,
  type CreateQiweiAccountInput,
  type QiweiAccount,
  type UpdateQiweiAccountInput,
  type UnknownGuidEvent,
} from "@/api/qiwei-accounts"

const ACCOUNTS_KEY = ["qiwei-accounts"] as const
const UNKNOWN_KEY = ["qiwei-unknown-guids"] as const

export function useQiweiAccounts() {
  return useQuery<QiweiAccount[]>({
    queryKey: ACCOUNTS_KEY,
    queryFn: async () => {
      const res = await qiweiAccountsApi.list()
      return res.data ?? []
    },
  })
}

export function useQiweiAccount(id: string | undefined) {
  return useQuery<QiweiAccount | undefined>({
    queryKey: [...ACCOUNTS_KEY, id],
    queryFn: async () => {
      const res = await qiweiAccountsApi.get(id!)
      return res.data
    },
    enabled: !!id,
  })
}

export function useCreateQiweiAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateQiweiAccountInput) => qiweiAccountsApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ACCOUNTS_KEY })
    },
  })
}

export function useUpdateQiweiAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateQiweiAccountInput }) =>
      qiweiAccountsApi.update(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ACCOUNTS_KEY })
    },
  })
}

export function useSoftDeleteQiweiAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => qiweiAccountsApi.softDelete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ACCOUNTS_KEY })
    },
  })
}

export function useHardDeleteQiweiAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => qiweiAccountsApi.hardDelete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ACCOUNTS_KEY })
    },
  })
}

export function useRefreshQiweiProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => qiweiAccountsApi.refreshProfile(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ACCOUNTS_KEY })
    },
  })
}

export function useReloadQiweiRegistry() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => qiweiAccountsApi.reload(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ACCOUNTS_KEY })
    },
  })
}

export function useUnknownQiweiGuids() {
  return useQuery<UnknownGuidEvent[]>({
    queryKey: UNKNOWN_KEY,
    queryFn: async () => {
      const res = await qiweiAccountsApi.unknownGuids()
      return res.data ?? []
    },
    refetchInterval: 10_000,
  })
}
