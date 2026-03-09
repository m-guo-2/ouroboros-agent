import { useQuery } from "@tanstack/react-query"
import { tracesApi } from "@/api/traces"

export function useLLMIO(traceId: string, ref: string) {
  return useQuery({
    queryKey: ["llm-io", traceId, ref],
    queryFn: () => tracesApi.getLLMIO(traceId, ref),
    enabled: !!traceId && !!ref,
    staleTime: Infinity,
  })
}
