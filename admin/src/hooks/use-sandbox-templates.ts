import { useQuery } from "@tanstack/react-query"
import { sandboxTemplatesApi } from "@/api/sandbox-templates"

export function useSandboxTemplates() {
  return useQuery({
    queryKey: ["sandboxTemplates"],
    queryFn: async () => {
      const res = await sandboxTemplatesApi.list()
      return res.data ?? []
    },
    staleTime: 5 * 60 * 1000,
  })
}
