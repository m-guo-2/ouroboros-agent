import { Skeleton } from "@/components/ui/skeleton"

export function ExchangeSkeleton() {
  return (
    <div className="px-5 py-3 space-y-3">
      <div className="flex gap-3">
        <Skeleton className="h-7 w-7 rounded-full flex-shrink-0" />
        <div className="flex-1 space-y-1.5">
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-4 w-3/4" />
        </div>
      </div>
      <Skeleton className="h-8 w-full rounded-md ml-10" />
      <div className="flex gap-3">
        <Skeleton className="h-7 w-7 rounded-full flex-shrink-0" />
        <div className="flex-1 space-y-1.5">
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-12 w-full rounded-md" />
        </div>
      </div>
    </div>
  )
}
