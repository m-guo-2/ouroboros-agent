import { Badge } from "@/components/ui/badge"
import { channelLabel } from "@/lib/utils"

export function ChannelBadge({ channel }: { channel: string }) {
  return <Badge variant="outline">{channelLabel(channel)}</Badge>
}
