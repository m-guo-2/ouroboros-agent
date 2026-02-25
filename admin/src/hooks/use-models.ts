import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { modelsApi } from "@/api/models"
import type { Model } from "@/api/types"

export function useModels() {
  return useQuery({
    queryKey: ["models"],
    queryFn: async () => {
      const res = await modelsApi.getAll()
      return res.data ?? []
    },
  })
}

export function useUpdateModel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Model> & { apiKey?: string } }) =>
      modelsApi.update(id, data),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["models"] }) },
  })
}

export function useAvailableModels(id: string | undefined) {
  return useQuery({
    queryKey: ["models", id, "available"],
    queryFn: async () => {
      const res = await modelsApi.getAvailableModels(id!)
      return res.data ?? []
    },
    enabled: false, // manual trigger only
  })
}
