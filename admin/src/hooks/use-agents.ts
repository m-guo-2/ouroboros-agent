import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { agentsApi } from "@/api/agents"
import type { AgentProfile } from "@/api/types"

export function useAgents() {
  return useQuery({
    queryKey: ["agents"],
    queryFn: async () => {
      const res = await agentsApi.getAll()
      return res.data ?? []
    },
  })
}

export function useAgent(id: string | undefined) {
  return useQuery({
    queryKey: ["agents", id],
    queryFn: async () => {
      const res = await agentsApi.getById(id!)
      return res.data
    },
    enabled: !!id,
  })
}

export function useCreateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: agentsApi.create,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["agents"] }) },
  })
}

export function useUpdateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<AgentProfile> }) => agentsApi.update(id, data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ["agents"] })
      qc.invalidateQueries({ queryKey: ["agents", variables.id] })
    },
  })
}

export function useDeleteAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: agentsApi.delete,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["agents"] }) },
  })
}
