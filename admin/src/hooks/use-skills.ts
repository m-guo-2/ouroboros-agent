import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { skillsApi } from "@/api/skills"

export function useSkills() {
  return useQuery({
    queryKey: ["skills"],
    queryFn: async () => {
      const res = await skillsApi.getAll()
      return res.data ?? []
    },
  })
}

export function useSkill(id: string | undefined) {
  return useQuery({
    queryKey: ["skills", id],
    queryFn: async () => {
      const res = await skillsApi.getById(id!)
      return res.data
    },
    enabled: !!id,
  })
}

export function useCreateSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Record<string, unknown>) => skillsApi.create(data),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["skills"] }) },
  })
}

export function useUpdateSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string;[key: string]: unknown }) =>
      skillsApi.update(id, data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ["skills"] })
      qc.invalidateQueries({ queryKey: ["skills", variables.id] })
      qc.invalidateQueries({ queryKey: ["skill-versions", variables.id] })
    },
  })
}

export function useToggleSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => skillsApi.toggle(id, enabled),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["skills"] }) },
  })
}

export function useDeleteSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: skillsApi.delete,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["skills"] }) },
  })
}

export function useSkillVersions(id: string | undefined) {
  return useQuery({
    queryKey: ["skill-versions", id],
    queryFn: async () => {
      const res = await skillsApi.getVersions(id!)
      return res.data ?? []
    },
    enabled: !!id,
  })
}

export function useSkillVersion(id: string | undefined, version: number | undefined) {
  return useQuery({
    queryKey: ["skill-versions", id, version],
    queryFn: async () => {
      const res = await skillsApi.getVersion(id!, version!)
      return res.data
    },
    enabled: !!id && version !== undefined,
  })
}

export function useRestoreSkillVersion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, version }: { id: string; version: number }) =>
      skillsApi.restoreVersion(id, version),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ["skills"] })
      qc.invalidateQueries({ queryKey: ["skills", variables.id] })
      qc.invalidateQueries({ queryKey: ["skill-versions", variables.id] })
    },
  })
}
