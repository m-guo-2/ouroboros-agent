import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { personasApi, groupAssignmentsApi } from "@/api/personas"
import type { Persona } from "@/api/types"

export function usePersonas(agentId: string | undefined) {
  return useQuery({
    queryKey: ["personas", agentId],
    queryFn: async () => {
      const res = await personasApi.list(agentId!)
      return res.data ?? []
    },
    enabled: !!agentId,
  })
}

export function usePersona(agentId: string | undefined, personaId: string | undefined) {
  return useQuery({
    queryKey: ["personas", agentId, personaId],
    queryFn: async () => {
      const res = await personasApi.get(agentId!, personaId!)
      return res.data
    },
    enabled: !!agentId && !!personaId,
  })
}

export function useCreatePersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentId, data }: { agentId: string; data: Partial<Persona> }) =>
      personasApi.create(agentId, data),
    onSuccess: (_res, variables) => {
      qc.invalidateQueries({ queryKey: ["personas", variables.agentId] })
    },
  })
}

export function useUpdatePersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      agentId,
      id,
      data,
    }: {
      agentId: string
      id: string
      data: Partial<Persona>
    }) => personasApi.update(agentId, id, data),
    onSuccess: (_res, variables) => {
      qc.invalidateQueries({ queryKey: ["personas", variables.agentId] })
      qc.invalidateQueries({ queryKey: ["personas", variables.agentId, variables.id] })
      qc.invalidateQueries({ queryKey: ["groupAssignments", variables.agentId] })
    },
  })
}

export function useClonePersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      agentId,
      id,
      displayName,
    }: {
      agentId: string
      id: string
      displayName: string
    }) => personasApi.clone(agentId, id, displayName),
    onSuccess: (_res, variables) => {
      qc.invalidateQueries({ queryKey: ["personas", variables.agentId] })
    },
  })
}

export function useDeletePersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentId, id }: { agentId: string; id: string }) =>
      personasApi.delete(agentId, id),
    onSuccess: (_res, variables) => {
      qc.invalidateQueries({ queryKey: ["personas", variables.agentId] })
      qc.invalidateQueries({ queryKey: ["groupAssignments", variables.agentId] })
    },
  })
}

export function useGroupAssignments(agentId: string | undefined) {
  return useQuery({
    queryKey: ["groupAssignments", agentId],
    queryFn: async () => {
      const res = await groupAssignmentsApi.list(agentId!)
      return res.data ?? []
    },
    enabled: !!agentId,
  })
}

export function useUnconfiguredGroups(agentId: string | undefined) {
  return useQuery({
    queryKey: ["unconfiguredGroups", agentId],
    queryFn: async () => {
      const res = await groupAssignmentsApi.discover(agentId!)
      return res.data ?? []
    },
    enabled: !!agentId,
  })
}

export function useCreateGroupAssignment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      agentId,
      data,
    }: {
      agentId: string
      data: { sessionKey: string; groupName: string; personaId?: string }
    }) => groupAssignmentsApi.create(agentId, data),
    onSuccess: (_res, variables) => {
      qc.invalidateQueries({ queryKey: ["groupAssignments", variables.agentId] })
      qc.invalidateQueries({ queryKey: ["unconfiguredGroups", variables.agentId] })
    },
  })
}

export function useUpdateGroupAssignment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      agentId,
      id,
      data,
    }: {
      agentId: string
      id: string
      data: { personaId?: string; groupName?: string }
    }) => groupAssignmentsApi.update(agentId, id, data),
    onSuccess: (_res, variables) => {
      qc.invalidateQueries({ queryKey: ["groupAssignments", variables.agentId] })
    },
  })
}

export function useDeleteGroupAssignment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentId, id }: { agentId: string; id: string }) =>
      groupAssignmentsApi.delete(agentId, id),
    onSuccess: (_res, variables) => {
      qc.invalidateQueries({ queryKey: ["groupAssignments", variables.agentId] })
      qc.invalidateQueries({ queryKey: ["unconfiguredGroups", variables.agentId] })
    },
  })
}
