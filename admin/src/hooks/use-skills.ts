import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { skillsApi } from "@/api/skills"
import type { SkillManifest } from "@/api/types"

export function useSkills() {
  return useQuery({
    queryKey: ["skills"],
    queryFn: async () => {
      const res = await skillsApi.getAll()
      return res.data ?? []
    },
  })
}

export function useSkill(name: string | undefined) {
  return useQuery({
    queryKey: ["skills", name],
    queryFn: async () => {
      const res = await skillsApi.getById(name!)
      return res.data
    },
    enabled: !!name,
  })
}

export function useCreateSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, manifest, readme }: { name: string; manifest: Omit<SkillManifest, "version">; readme?: string }) =>
      skillsApi.create(name, manifest, readme),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["skills"] }) },
  })
}

export function useUpdateSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, manifest, readme, changeSummary }: { name: string; manifest?: Partial<SkillManifest>; readme?: string; changeSummary?: string }) =>
      skillsApi.update(name, manifest, readme, changeSummary),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ["skills"] })
      qc.invalidateQueries({ queryKey: ["skills", variables.name] })
      qc.invalidateQueries({ queryKey: ["skill-versions", variables.name] })
    },
  })
}

export function useToggleSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, enabled }: { name: string; enabled: boolean }) => skillsApi.toggle(name, enabled),
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

export function useSkillVersions(name: string | undefined) {
  return useQuery({
    queryKey: ["skill-versions", name],
    queryFn: async () => {
      const res = await skillsApi.getVersions(name!)
      return res.data ?? []
    },
    enabled: !!name,
  })
}

export function useSkillVersion(name: string | undefined, version: number | undefined) {
  return useQuery({
    queryKey: ["skill-versions", name, version],
    queryFn: async () => {
      const res = await skillsApi.getVersion(name!, version!)
      return res.data
    },
    enabled: !!name && version !== undefined,
  })
}

export function useRestoreSkillVersion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, version }: { name: string; version: number }) =>
      skillsApi.restoreVersion(name, version),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ["skills"] })
      qc.invalidateQueries({ queryKey: ["skills", variables.name] })
      qc.invalidateQueries({ queryKey: ["skill-versions", variables.name] })
    },
  })
}
