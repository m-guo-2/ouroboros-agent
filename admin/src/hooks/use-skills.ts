import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { skillsApi } from "@/api/skills"
import type { ImportParams } from "@/api/skills"

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

export function useRefreshSkills() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => skillsApi.refresh(),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["skills"] }) },
  })
}

export function useBrowseImport() {
  return useMutation({
    mutationFn: (url: string) => skillsApi.browseImport(url),
  })
}

export function useImportSkills() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (params: ImportParams) => skillsApi.importSkills(params),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["skills"] }) },
  })
}
